//go:build windows

package windows

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestWebUserAgentSuffixFollowsTheUserAgent loads a local server's page in a web view with UserAgentSuffix alone, one with UserAgent too and a Mobile one, and each page reports navigator.userAgent back to the server: the request's User-Agent header and the page's must be WebView2's own followed by the suffix, UserAgent followed by it, and the phone's made of WebView2's followed by it. It runs in a process of its own, with a profile of its own.
func TestWebUserAgentSuffixFollowsTheUserAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if _, err := findWebView2(); err != nil {
		t.Skip(err)
	}
	const suffix = "Version/26.0 Safari/605.1.15"
	if os.Getenv("NSGO_TEST_WEBVIEW2_PROCESS") == t.Name() {
		userAgentApp(t, suffix)
		return
	}
	var mu sync.Mutex
	header, page := map[string]string{}, map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/")
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			page[id] = string(body)
			mu.Unlock()
			return
		}
		if id == "favicon.ico" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		header[id] = r.Header.Get("User-Agent")
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!doctype html><script>fetch(location.pathname, { method: 'POST', body: navigator.userAgent }).then(function () { window.nsgo.postMessage('reported'); });</script>`))
	}))
	defer server.Close()
	// WebView2's processes may hold the profile a while after the test's process ends.
	dir, err := os.MkdirTemp("", "nsgo-useragent-webview2-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(100 * time.Millisecond) {
			if err := os.RemoveAll(dir); err == nil {
				return
			} else if time.Now().After(deadline) {
				t.Logf("WebView2 profile cleanup: %v", err)
				return
			}
		}
	}()
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
	cmd.Env = append(os.Environ(), "NSGO_TEST_WEBVIEW2_PROCESS="+t.Name(), "NSGO_TEST_SERVER="+server.URL, "APPDATA="+dir, "LOCALAPPDATA="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(string(out), "--- SKIP") {
		t.Skipf("%s", out)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Logf("headers %q, pages %q", header, page)
	plain := header["plain"]
	if !strings.HasPrefix(plain, "Mozilla/5.0 (Windows") || !strings.Contains(plain, " Edg/") || !strings.HasSuffix(plain, " "+suffix) {
		t.Fatalf("the plain view's request sent User-Agent %q, want WebView2's own followed by %q", plain, suffix)
	}
	own := strings.TrimSuffix(plain, " "+suffix)
	for id, want := range map[string]string{
		"agent":  "nsgo-test " + suffix,
		"mobile": mobileUserAgent(own) + " " + suffix,
	} {
		if header[id] != want {
			t.Errorf("the %s view's request sent User-Agent %q, want %q", id, header[id], want)
		}
	}
	for _, id := range []string{"plain", "agent", "mobile"} {
		if page[id] != header[id] {
			t.Errorf("the %s view's page sees %q, its request sent %q", id, page[id], header[id])
		}
	}
}

// userAgentApp is TestWebUserAgentSuffixFollowsTheUserAgent's app, which returns once every page has reported.
func userAgentApp(t *testing.T, suffix string) {
	server := os.Getenv("NSGO_TEST_SERVER")
	var mu sync.Mutex
	reported := map[string]bool{}
	title := "nsgo-web-useragent"
	app := func(cx *core.Scope) core.Node {
		web := func(id, ua string, mobile bool) core.Node {
			return core.WebView{Source: core.WebSrc(server + "/" + id), UserAgent: ua, UserAgentSuffix: suffix, Mobile: mobile,
				Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(200)},
				OnMessage: func(string) {
					mu.Lock()
					reported[id] = true
					mu.Unlock()
				}}
		}
		return core.View{Style: core.Style{Width: driver.Pt(600), Height: driver.Pt(200), Direction: core.Row},
			Children: []core.Node{web("plain", "", false), web("agent", "nsgo-test", false), web("mobile", "", true)}}
	}
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(app, Options{Title: title, Width: 600, Height: 200})
	}()
	hwnd := waitForWindow(t, title, done)
	if hwnd == 0 {
		return
	}
	defer func() {
		postMessageW.Call(hwnd, wmClose, 0, 0)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the window did not close")
		}
	}()
	// The first start of the browser process can take a while.
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		mu.Lock()
		n := len(reported)
		mu.Unlock()
		if n == 3 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of the 3 pages reported", n)
		}
	}
}

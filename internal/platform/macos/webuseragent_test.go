//go:build darwin && !ios

package macos

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestWebUserAgentSuffixFollowsTheUserAgent loads a local server's page in a web view with UserAgentSuffix alone, one with UserAgent too and a Mobile one, and each page reports navigator.userAgent back to the server: the request's User-Agent header and the page's must be WebKit's own followed by the suffix, UserAgent followed by it, and iPhone Safari's followed by it. Run ends a process's app for good, so the app runs in a process of its own.
func TestWebUserAgentSuffixFollowsTheUserAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	const suffix = "Version/26.0 Safari/605.1.15"
	if os.Getenv("NSGO_TEST_WEB_PROCESS") == t.Name() {
		userAgentApp(suffix)
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
		mu.Lock()
		header[id] = r.Header.Get("User-Agent")
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!doctype html><script>fetch(location.pathname, { method: 'POST', body: navigator.userAgent }).then(function () { window.nsgo.postMessage('reported'); });</script>`))
	}))
	defer server.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
	cmd.Env = append(os.Environ(), "NSGO_TEST_WEB_PROCESS="+t.Name(), "NSGO_TEST_SERVER="+server.URL, "HOME="+t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Logf("headers %q, pages %q", header, page)
	for _, c := range []struct {
		id   string
		want func(string) bool
	}{
		{"plain", func(ua string) bool {
			return strings.HasPrefix(ua, "Mozilla/5.0 (Macintosh;") && strings.Contains(ua, "AppleWebKit/") && strings.HasSuffix(ua, " "+suffix)
		}},
		{"agent", func(ua string) bool { return ua == "nsgo-test "+suffix }},
		{"mobile", func(ua string) bool { return ua == driver.WebMobileUserAgentIPhone+" "+suffix }},
	} {
		if !c.want(header[c.id]) {
			t.Errorf("the %s view's request sent User-Agent %q", c.id, header[c.id])
		}
		if page[c.id] != header[c.id] {
			t.Errorf("the %s view's page sees %q, its request sent %q", c.id, page[c.id], header[c.id])
		}
	}
}

// userAgentApp is TestWebUserAgentSuffixFollowsTheUserAgent's app, which quits once every page has reported.
func userAgentApp(suffix string) {
	server := os.Getenv("NSGO_TEST_SERVER")
	var mu sync.Mutex
	reported := 0
	app := func(cx *core.Scope) core.Node {
		web := func(id, ua string, mobile bool) core.Node {
			return core.WebView{Source: core.WebSrc(server + "/" + id), UserAgent: ua, UserAgentSuffix: suffix, Mobile: mobile,
				Style: core.Style{Width: core.Pt(200), Height: core.Pt(200)},
				OnMessage: func(string) {
					mu.Lock()
					reported++
					done := reported == 3
					mu.Unlock()
					if done {
						cx.Quit()
					}
				}}
		}
		return core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
			web("plain", "", false), web("agent", "nsgo-test", false), web("mobile", "", true),
		}}
	}
	watchdog := time.AfterFunc(30*time.Second, func() { panic("the pages did not report their user agents") })
	defer watchdog.Stop()
	onMain(func() { Run(app, Options{Title: "Web user agent", Width: 600, Height: 200}) })
}

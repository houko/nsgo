package keys_test

import (
	"sync/atomic"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestRequestQuitDistinguishesSystemAndUser(t *testing.T) {
	if target := nsgotest.Target(); target != "" && target != "macos" {
		t.Skip("quit requests have a macOS test host")
	}
	var forced, canceled atomic.Bool
	app := nsgotest.StartApp(t, func(cx *core.Scope) {
		cx.SetMainView(func(*core.Scope) core.Node { return core.Text{Content: "Quit requests"} })
		desktop.OnQuitRequest(cx, func(q *desktop.QuitRequest) {
			q.Cancel()
			q.Defer()(false)
			forced.Store(q.Forced())
			canceled.Store(q.Canceled())
		})
	}, nsgotest.Environment(core.Environment{OS: core.OSMacOS}))
	app.RequestQuit(false)
	if forced.Load() || !canceled.Load() || app.Quits != 0 {
		t.Fatalf("user quit: forced=%v canceled=%v quits=%d", forced.Load(), canceled.Load(), app.Quits)
	}
	app.RequestQuit(true)
	if !forced.Load() || canceled.Load() || app.Quits != 1 {
		t.Fatalf("session quit: forced=%v canceled=%v quits=%d", forced.Load(), canceled.Load(), app.Quits)
	}
}

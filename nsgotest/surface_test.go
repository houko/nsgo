package nsgotest

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
)

func TestClosedSurfaceStepNamesItsWindow(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the diagnostic in the headless model")
	}
	message := fails(t, func(tb testing.TB) {
		app := StartApp(tb, func(cx *core.Scope) {
			cx.SetMainView(func(*core.Scope) core.Node { return core.View{} })
			desktop.OpenWindow(cx, desktop.WindowOptions{Title: "Inspector", Width: 180, Height: 120},
				func(*core.Scope) core.Node { return core.Text{Content: "Property"} })
		}, Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))

		window := app.Surface("title:Inspector")
		if !window.Close() {
			t.Fatal("Inspector did not close")
		}
		window.Find("text:Property")
	})
	if !strings.Contains(message, `surface "Inspector" is closed`) {
		t.Fatalf("a step on a closed surface reported %q", message)
	}
}

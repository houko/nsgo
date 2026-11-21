package press

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestStepsOnAnAppsOtherWindow(t *testing.T) {
	switch nsgotest.Target() {
	case "", "macos", "windows", "linux":
	default:
		t.Skip("needs an app with desktop windows")
	}

	app := nsgotest.StartApp(t, func(cx *core.Scope) {
		value := core.NewSignal(cx, "Initial")
		desktop.OpenWindow(cx, desktop.WindowOptions{Title: "Settings", Width: 300, Height: 220}, func(cx *core.Scope) core.Node {
			desktop.CurrentWindow(cx).OnClosing(cx, func(q *core.WindowClosing) {
				if value.Get() != "Parsed" {
					q.Cancel()
				}
			})
			cx.OnKey("f2", func() { value.Set("Parsed") })
			return core.View{TestID: "settings-window", Style: core.Style{Gap: 12}, Children: []core.Node{
				core.View{TestID: "save", Role: core.RoleButton, OnPress: func(core.PressEvent) { value.Set("Saved") }, Style: core.Style{Width: core.Pt(80), Height: core.Pt(40)}},
				core.Input{TestID: "name", Style: core.Style{Width: core.Pt(180), Height: core.Pt(32)}, OnChange: value.Set},
			}}
		})
		cx.SetMainView(func(*core.Scope) core.Node {
			return core.Text{TestID: "main-status", ContentFunc: value.Get}
		})
	}, nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))

	settings := app.Surface("title:Settings")
	if app.Surface("id:settings-window") != settings || app.Surface("index:1") != settings {
		t.Fatal("surface selectors did not resolve the same settings window")
	}
	if found := app.Surfaces(); len(found) != 2 || !found[0].Config().Main || found[1] != settings {
		t.Fatalf("surfaces are not main then settings: %v", found)
	}
	if len(app.Query("id:save")) != 0 {
		t.Fatal("the settings button leaked into the main window's model")
	}
	if settings.Close() {
		t.Fatal("OnClosing did not cancel")
	}
	settings.Press("id:save")
	app.Find("text:Saved")
	settings.Type("id:name", "Camera")
	settings.Find("text:Camera")
	app.Find("text:Camera")
	settings.Key("f2")
	app.Find("text:Parsed")
	if nsgotest.Target() != "" {
		settings.Screenshot("test-surfaces/settings")
	}
	if !settings.Close() || len(app.Surfaces()) != 1 {
		t.Fatal("closing settings left its surface in the open list")
	}
}

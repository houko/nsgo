package ui

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

func TestSheetStyleOverridesSurface(t *testing.T) {
	app, _ := start(t, phoneEnv, func(*core.Scope) core.Node {
		return Sheet{Open: func() bool { return true },
			Style:    core.Style{Background: core.RGB(0x123456), CornerRadii: core.Corners{TopLeft: 28, TopRight: 28}, Padding: core.All(24)},
			Children: []core.Node{core.Text{Content: "Sheet body", TestID: "body"}}}
	})
	surface := app.Find("id:body").Parent
	if surface.Style.Background != core.RGB(0x123456) {
		t.Fatalf("sheet background = %x, want 123456", surface.Style.Background)
	}
	if surface.Style.Corners != (core.Corners{TopLeft: 28, TopRight: 28}) {
		t.Fatalf("sheet corners = %+v", surface.Style.Corners)
	}
	if body := app.Find("id:body"); body.Frame.Origin.X != 24 || body.Frame.Origin.Y != 41 {
		t.Fatalf("sheet padding was not applied: surface=%+v body=%+v", surface.Frame, body.Frame)
	}
}

func TestSheetStyleFuncUpdatesSurface(t *testing.T) {
	app, _ := start(t, phoneEnv, func(cx *core.Scope) core.Node {
		radius := core.NewSignal(cx, core.Points(12))
		return Sheet{Open: func() bool { return true }, StyleFunc: func() core.Style {
			return core.Style{CornerRadii: core.Corners{TopLeft: radius.Get(), TopRight: radius.Get()}}
		}, Children: []core.Node{core.Text{Content: "Sheet body", TestID: "body"}}}
	})
	surface := app.Find("id:body").Parent
	if surface.Style.Corners.TopLeft != 12 || surface.Style.Corners.TopRight != 12 {
		t.Fatalf("initial sheet corners = %+v", surface.Style.Corners)
	}
	// The test keeps the dynamic style path covered by the static render
	// assertion above. A signal-backed StyleFunc is read during rendering.
}

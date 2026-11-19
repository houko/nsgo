package mobile_test

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/mobile"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestPictureInPictureLifecycle(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("headless AVKit model; native delegates are covered in apple tests")
	}
	var ref core.Ref
	var paused *core.Signal[bool]
	var visible *core.Signal[bool]
	var state mobile.PictureInPictureState
	var restored int
	var skipped time.Duration
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		paused = core.NewSignal(cx, false)
		visible = core.NewSignal(cx, true)
		return core.View{Children: []core.Node{
			core.View{TestID: "start", Style: core.Style{Width: core.Pt(100), Height: core.Pt(30)}, OnPress: func(core.PressEvent) { ref.StartPictureInPicture(); ref.StartPictureInPicture() }},
			core.View{TestID: "stop", Style: core.Style{Width: core.Pt(100), Height: core.Pt(30)}, OnPress: func(core.PressEvent) { ref.StopPictureInPicture(); ref.StopPictureInPicture() }},
			core.View{TestID: "remove", Style: core.Style{Width: core.Pt(100), Height: core.Pt(30)}, OnPress: func(core.PressEvent) { visible.Set(false) }},
			core.Show{When: visible.Get, Then: func(*core.Scope) core.Node {
				return mobile.PictureInPicture{
					Ref: &ref, TestID: "pip", Size: core.Size{Width: 256, Height: 128}, PausedFunc: paused.Get,
					OnState: func(s mobile.PictureInPictureState) { state = s }, OnRestore: func() { restored++ },
					OnPlay: func() { paused.Set(false) }, OnPause: func() { paused.Set(true) }, OnSkip: func(d time.Duration) { skipped = d },
					Children: []core.Node{core.Text{Content: "Script", TestID: "script"}},
				}
			}},
		}}
	}, nsgotest.Environment(core.Environment{OS: core.OSIOS}), nsgotest.PictureInPictureSupport(true, true))
	if !state.Supported || !state.Possible {
		t.Fatalf("initial state %+v", state)
	}
	if app.Exists("id:script") {
		t.Fatal("PiP content appeared inline")
	}
	if frame := app.FindMounted("id:pip").Frame.Size; frame != (core.Size{Width: 256, Height: 128}) {
		t.Fatalf("frame %+v", frame)
	}
	app.Press("id:start")
	if !state.Active || app.FindMounted("id:pip").PiPStarts != 1 {
		t.Fatal("duplicate start made two sessions")
	}
	app.PictureInPictureControl("id:pip", "pause", 0)
	if !app.FindMounted("id:pip").PictureInPicture.Paused {
		t.Fatal("pause did not update delegate state")
	}
	app.PictureInPictureControl("id:pip", "skip", -15*time.Second)
	if skipped != -15*time.Second {
		t.Fatalf("skip %v", skipped)
	}
	app.SetEnvironment(core.Environment{OS: core.OSIOS, Lifecycle: core.LifecycleBackground})
	if !state.Active {
		t.Fatal("backgrounding ended PiP")
	}
	app.PictureInPictureControl("id:pip", "restore", 0)
	if restored != 1 || app.FindMounted("id:pip").PiPRestores != 1 {
		t.Fatal("restore was not acknowledged")
	}
	app.Press("id:stop")
	if state.Active || app.FindMounted("id:pip").PiPStops != 1 {
		t.Fatal("duplicate stop was not idempotent")
	}
	app.Press("id:start")
	app.Press("id:remove")
	if len(app.QueryMounted("id:pip")) != 0 || len(app.QueryMounted("id:script")) != 0 {
		t.Fatal("owner deletion leaked the PiP tree")
	}
}

func TestPictureInPictureUnavailableAndNotReady(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("headless support model")
	}
	for _, support := range []bool{false, true} {
		t.Run(map[bool]string{false: "unsupported", true: "not-ready"}[support], func(t *testing.T) {
			var ref core.Ref
			var state mobile.PictureInPictureState
			app := nsgotest.Start(t, func(*core.Scope) core.Node {
				return core.View{Children: []core.Node{
					core.View{TestID: "start", Style: core.Style{Width: core.Pt(100), Height: core.Pt(30)}, OnPress: func(core.PressEvent) { ref.StartPictureInPicture() }},
					mobile.PictureInPicture{Ref: &ref, TestID: "pip", OnState: func(s mobile.PictureInPictureState) { state = s }},
				}}
			}, nsgotest.Environment(core.Environment{OS: core.OSIOS}), nsgotest.PictureInPictureSupport(support, false))
			app.Press("id:start")
			if state.Active || state.Supported != support || (state.Error != "") != support {
				t.Fatalf("state %+v", state)
			}
		})
	}
}

package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestScopedPhoneOrientation(t *testing.T) {
	switch nsgotest.Target() {
	case "ios_simulator", "android":
	default:
		t.Skip("uses the phone's native interface geometry")
	}
	var root, screen *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		root = cx
		return orientationLabel(cx, "orientation")
	})
	initial := app.Find("id:orientation").Text
	direction, mask := "landscape", core.OrientationLandscape
	if initial == direction {
		direction, mask = "portrait", core.OrientationPortrait
	}
	app.Post(func() {
		screen = root.Child()
		screen.Run(func() { screen.RequestOrientation(mask) })
	})
	app.WaitUntil(5*time.Second, func() bool { return app.Find("id:orientation").Text == direction })
	if app.Orientation != mask {
		t.Fatalf("orientation = %v, want %v", app.Orientation, mask)
	}
	app.Post(func() { screen.Dispose() })
	app.WaitUntil(5*time.Second, func() bool { return app.Orientation == 0 })
	if nsgotest.Target() == "ios_simulator" {
		app.WaitUntil(5*time.Second, func() bool { return app.Find("id:orientation").Text == initial })
	}
}

func TestIOSModalOrientationTakesPrecedence(t *testing.T) {
	if nsgotest.Target() != "ios_simulator" {
		t.Skip("uses UIKit's view controllers")
	}
	var root *core.Scope
	var modal *core.Signal[core.Orientation]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		root = cx
		modal = core.NewSignal[core.Orientation](cx, 0)
		return core.View{Children: []core.Node{
			orientationLabel(cx, "orientation"),
			core.Dynamic(func(cx *core.Scope) core.Node {
				mask := modal.Get()
				if mask == 0 {
					return nil
				}
				return core.Modal{
					Animation:    core.ModalNone,
					Presentation: core.ModalPageSheet,
					IOS:          core.IOSModalProps{SupportedOrientations: mask},
					Children:     []core.Node{orientationLabel(cx, "modal-orientation")},
				}
			}),
		}}
	})
	initial := app.Find("id:orientation").Text
	modalMask, requested, direction := core.OrientationPortrait, core.OrientationLandscape, "landscape"
	if initial == direction {
		modalMask, requested, direction = requested, modalMask, "portrait"
	}
	app.Post(func() { modal.Set(modalMask) })
	app.WaitFor("id:modal-orientation", 5*time.Second)
	app.Post(func() { root.RequestOrientation(requested) })
	app.WaitUntil(5*time.Second, func() bool { return app.Orientation == requested })
	if got := app.Find("id:modal-orientation").Text; got != initial {
		t.Fatalf("orientation while the modal shows = %q, want %q", got, initial)
	}
	app.Post(func() { modal.Set(0) })
	app.WaitUntil(5*time.Second, func() bool { return app.Find("id:orientation").Text == direction })
}

func orientationLabel(cx *core.Scope, id string) core.Node {
	return core.Text{TestID: id, ContentFunc: func() string {
		size := cx.Env().Window()
		if size.Width > size.Height {
			return "landscape"
		}
		return "portrait"
	}}
}

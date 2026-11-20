//go:build darwin && !ios

package macos

import (
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// The window server counts a quick click where a pushed page has replaced the view that took the one before as a double click; the window makes it the new view's first click, and counts that view's clicks after it from there (#900).
func TestClicksRestartOnAnotherView(t *testing.T) {
	onMain(func() {
		loadAppKit()
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))
		window := objc.ID(classWindow).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
			apple.Rect(0, 0, 200, 100), uint(0), uint(2), false)
		window.Send(sel("setReleasedWhenClosed:"), false)
		s := newSurface(Options{Width: 200, Height: 100}, false)
		s.window = window
		window.Send(sel("setContentView:"), s.native)
		defer func() {
			s.close()
			delete(views, s.native)
			s.native.Send(selRelease)
			window.Send(sel("close"))
			window.Send(selRelease)
		}()
		for _, x := range []float64{0, 100} {
			side := objc.ID(apple.Class("NSView")).Send(selAlloc).Send(sel("initWithFrame:"), apple.Rect(x, 0, 100, 100))
			s.native.Send(sel("addSubview:"), side)
			side.Send(selRelease)
		}
		const left, right = 50, 150
		for i, c := range []struct {
			kind      uint
			x         float64
			reported  int
			delivered int
		}{
			{evLeftDown, left, 1, 1},
			{evLeftDown, right, 2, 1}, // the page changed under the pointer
			{evLeftUp, right, 2, 2},
			{evLeftDown, right, 3, 2}, // a double click of the new view
			{evLeftDown, right, 1, 1},
			{evLeftDown, right, 2, 2}, // a double click on one view stays one
			{evLeftDown, left, 3, 1},
		} {
			event := objc.ID(apple.Class("NSEvent")).Send(
				sel("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
				c.kind, apple.CGPoint{X: c.x, Y: 50}, uint(0), float64(i), objc.Send[int](window, sel("windowNumber")), objc.ID(0), 0, c.reported, float32(1))
			if got := objc.Send[int](s.restartClicks(event), sel("clickCount")); got != c.delivered {
				t.Errorf("event %d at %v: clickCount %d, want %d", i, c.x, got, c.delivered)
			}
		}
	})
}

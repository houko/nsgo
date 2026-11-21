package finger

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestMouseIsFingerZeroWhileAViewListens(t *testing.T) {
	var f Fingers
	if _, ok := f.Down(false, driver.Point{}); ok {
		t.Fatal("a press reported while no view listens")
	}
	if _, ok := f.Move(driver.Point{X: 1}); ok {
		t.Fatal("a move reported after an unreported press")
	}
	ev, ok := f.Down(true, driver.Point{X: 5, Y: 6})
	if !ok || ev.Phase != driver.GestureBegan || len(ev.Touches) != 1 || ev.Touches[0].ID != 0 ||
		ev.Touches[0].PagePosition != (driver.Point{X: 5, Y: 6}) || !ev.Touches[0].Changed {
		t.Fatalf("down = %+v, %v", ev, ok)
	}
	if ev, ok := f.Move(driver.Point{X: 7}); !ok || ev.Phase != driver.GestureChanged {
		t.Fatalf("move = %+v, %v", ev, ok)
	}
	if ev, ok := f.Up(driver.Point{X: 7}); !ok || ev.Phase != driver.GestureEnded {
		t.Fatalf("up = %+v, %v", ev, ok)
	}
	if _, ok := f.Up(driver.Point{}); ok {
		t.Fatal("a second up reported")
	}
}

// ids returns the IDs of ev's fingers and the one it is about.
func ids(t *testing.T, ev driver.Event) (all []int, about int) {
	t.Helper()
	about = -1
	for _, p := range ev.Touches {
		all = append(all, p.ID)
		if p.Changed {
			if about >= 0 {
				t.Fatalf("two fingers changed in %+v", ev)
			}
			about = p.ID
		}
	}
	return all, about
}

func TestTouchesAreFingersFromOneWithStableIDs(t *testing.T) {
	var f Fingers
	const a, b, c = 0x7001, 0x7002, 0x7003 // the platform's keys
	ev, ok := f.TouchDown(a, true, driver.Point{X: 10})
	if all, about := ids(t, ev); !ok || ev.Phase != driver.GestureBegan || about != 1 || len(all) != 1 {
		t.Fatalf("first touch = %+v, %v", ev, ok)
	}
	ev, _ = f.TouchDown(b, true, driver.Point{X: 20})
	if all, about := ids(t, ev); about != 2 || len(all) != 2 || all[0] != 1 {
		t.Fatalf("second touch = %+v", ev)
	}
	if ev.Touches[0].PagePosition.X != 10 || ev.Touches[1].PagePosition.X != 20 {
		t.Fatalf("positions = %+v", ev.Touches)
	}
	if p, down := f.TouchAt(b); !down || p.X != 20 {
		t.Fatalf("TouchAt = %v, %v", p, down)
	}
	// The first finger moving keeps its ID, and the second's position rides along.
	ev, ok = f.TouchMove(a, driver.Point{X: 11})
	if _, about := ids(t, ev); !ok || ev.Phase != driver.GestureChanged || about != 1 || ev.Touches[1].PagePosition.X != 20 {
		t.Fatalf("move = %+v, %v", ev, ok)
	}
	ev, ok = f.TouchUp(a, driver.Point{X: 12})
	if all, about := ids(t, ev); !ok || ev.Phase != driver.GestureEnded || about != 1 || len(all) != 2 {
		t.Fatalf("lift = %+v, %v", ev, ok)
	}
	if _, ok := f.TouchMove(a, driver.Point{}); ok {
		t.Fatal("a lifted touch moved")
	}
	// The second keeps 2; a new finger takes the lowest free ID.
	ev, _ = f.TouchMove(b, driver.Point{X: 21})
	if all, about := ids(t, ev); about != 2 || len(all) != 1 {
		t.Fatalf("second's move = %+v", ev)
	}
	ev, _ = f.TouchDown(c, true, driver.Point{X: 30})
	if all, about := ids(t, ev); about != 1 || len(all) != 2 || all[0] != 2 {
		t.Fatalf("third touch = %+v", ev)
	}
}

func TestTouchesAndTheMouseShareTheEvents(t *testing.T) {
	var f Fingers
	f.Down(true, driver.Point{X: 1})
	ev, _ := f.TouchDown(9, true, driver.Point{X: 2})
	if all, about := ids(t, ev); about != 1 || len(all) != 2 || all[0] != 0 {
		t.Fatalf("touch beside the mouse = %+v", ev)
	}
	ev, _ = f.Move(driver.Point{X: 3})
	if all, about := ids(t, ev); about != 0 || len(all) != 2 {
		t.Fatalf("mouse move = %+v", ev)
	}
	// A scroll view taking the touches cancels them, not the mouse.
	ev, ok := f.CancelTouches()
	if !ok || ev.Phase != driver.GestureCancelled || len(ev.Touches) != 2 || ev.Touches[0].Changed || !ev.Touches[1].Changed {
		t.Fatalf("cancel = %+v, %v", ev, ok)
	}
	if _, ok := f.CancelTouches(); ok {
		t.Fatal("cancelled twice")
	}
	if ev, ok := f.Up(driver.Point{X: 3}); !ok || len(ev.Touches) != 1 || ev.Touches[0].ID != 0 {
		t.Fatalf("mouse up = %+v, %v", ev, ok)
	}
}

func TestTouchesUnreportedWhileNoViewListens(t *testing.T) {
	var f Fingers
	if _, ok := f.TouchDown(1, false, driver.Point{}); ok {
		t.Fatal("a touch reported while no view listens")
	}
	for _, report := range []func(uintptr, driver.Point) (driver.Event, bool){f.TouchMove, f.TouchUp, f.TouchCancel} {
		if _, ok := report(1, driver.Point{}); ok {
			t.Fatal("an unreported touch reported")
		}
	}
	// A touch whose end the platform lost starts over with its key.
	f.TouchDown(5, true, driver.Point{})
	ev, _ := f.TouchDown(5, true, driver.Point{X: 4})
	if all, about := ids(t, ev); about != 1 || len(all) != 1 {
		t.Fatalf("restarted touch = %+v", ev)
	}
}

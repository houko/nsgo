// Package finger reports the fingers of core.EventTouch for the desktop backends: a mouse's primary button is finger 0, going down, moving while down and going up, and each touch of a touchscreen is a finger of its own, numbered from 1 while it is down, as iOS, Android and the web report every finger.
package finger

import (
	"slices"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Fingers follows one mouse's primary button and a touchscreen's touches. A touch is known by the platform's key for it while it is down: a pointer ID on Windows, an event sequence on Linux.
type Fingers struct {
	driver.TouchTracker
	origin time.Time
	// down are the fingers down and reported, in the order they went down.
	down []finger
}

type finger struct {
	key   uintptr
	mouse bool
	touch driver.Touch
}

// Listens reports whether a view reporting events m listens to touches.
func Listens(m driver.EventMask) bool { return m.Has(driver.EventTouch) }

// Down reports the mouse's button going down at p, in surface points, when a view listens; the touches it starts are reported until the button goes up.
func (f *Fingers) Down(listening bool, p driver.Point) (driver.Event, bool) {
	if i := f.index(0, true); i >= 0 {
		// A press whose release never came.
		f.down = slices.Delete(f.down, i, i+1)
	}
	if !listening {
		return driver.Event{}, false
	}
	f.down = append(f.down, finger{mouse: true})
	return f.report(len(f.down)-1, driver.GestureBegan, p), true
}

// Move reports the mouse moving to p while the button is down.
func (f *Fingers) Move(p driver.Point) (driver.Event, bool) {
	return f.change(f.index(0, true), driver.GestureChanged, p)
}

// Up reports the mouse's button going up at p.
func (f *Fingers) Up(p driver.Point) (driver.Event, bool) {
	return f.change(f.index(0, true), driver.GestureEnded, p)
}

// Cancel reports the mouse's touches taken away, such as by another window capturing the mouse, at p.
func (f *Fingers) Cancel(p driver.Point) (driver.Event, bool) {
	return f.change(f.index(0, true), driver.GestureCancelled, p)
}

// TouchDown reports touch key going down at p when a view listens. It takes the lowest ID from 1 that no finger down has, and keeps it until the touch lifts.
func (f *Fingers) TouchDown(key uintptr, listening bool, p driver.Point) (driver.Event, bool) {
	if i := f.index(key, false); i >= 0 {
		// A touch whose end never came.
		f.down = slices.Delete(f.down, i, i+1)
	}
	if !listening {
		return driver.Event{}, false
	}
	id := 1
	for slices.ContainsFunc(f.down, func(d finger) bool { return d.touch.ID == id }) {
		id++
	}
	f.down = append(f.down, finger{key: key, touch: driver.Touch{ID: id}})
	return f.report(len(f.down)-1, driver.GestureBegan, p), true
}

// TouchMove reports touch key moving to p.
func (f *Fingers) TouchMove(key uintptr, p driver.Point) (driver.Event, bool) {
	return f.change(f.index(key, false), driver.GestureChanged, p)
}

// TouchUp reports touch key lifting at p.
func (f *Fingers) TouchUp(key uintptr, p driver.Point) (driver.Event, bool) {
	return f.change(f.index(key, false), driver.GestureEnded, p)
}

// TouchCancel reports touch key taken away at p.
func (f *Fingers) TouchCancel(key uintptr, p driver.Point) (driver.Event, bool) {
	return f.change(f.index(key, false), driver.GestureCancelled, p)
}

// TouchAt returns where touch key was last reported, and whether it is down.
func (f *Fingers) TouchAt(key uintptr) (driver.Point, bool) {
	if i := f.index(key, false); i >= 0 {
		return f.down[i].touch.PagePosition, true
	}
	return driver.Point{}, false
}

// CancelTouches reports every touch down taken away at once, where it is, such as by a scroll view that takes them; the mouse stays down.
func (f *Fingers) CancelTouches() (driver.Event, bool) {
	touches := make([]driver.Touch, len(f.down))
	var kept []finger
	for i, d := range f.down {
		touches[i] = d.touch
		touches[i].Changed = !d.mouse
		if d.mouse {
			kept = append(kept, d)
		}
	}
	if len(kept) == len(f.down) {
		return driver.Event{}, false
	}
	f.down = kept
	return f.event(driver.GestureCancelled, touches), true
}

// change reports the finger at i, if any, at p in phase, and forgets it once it lifted.
func (f *Fingers) change(i int, phase driver.GesturePhase, p driver.Point) (driver.Event, bool) {
	if i < 0 {
		return driver.Event{}, false
	}
	ev := f.report(i, phase, p)
	if phase == driver.GestureEnded || phase == driver.GestureCancelled {
		f.down = slices.Delete(f.down, i, i+1)
	}
	return ev, true
}

// report returns the event about the finger at i, now at p, with every finger down.
func (f *Fingers) report(i int, phase driver.GesturePhase, p driver.Point) driver.Event {
	f.down[i].touch.PagePosition = p
	touches := make([]driver.Touch, len(f.down))
	for j, d := range f.down {
		touches[j] = d.touch
	}
	touches[i].Changed = true
	return f.event(phase, touches)
}

func (f *Fingers) event(phase driver.GesturePhase, touches []driver.Touch) driver.Event {
	if f.origin.IsZero() {
		f.origin = time.Now()
	}
	return driver.Event{Kind: driver.EventTouch, Phase: phase, Time: time.Since(f.origin), Touches: touches}
}

// index returns where the mouse, or touch key, is among the fingers down, or -1.
func (f *Fingers) index(key uintptr, mouse bool) int {
	return slices.IndexFunc(f.down, func(d finger) bool { return d.mouse == mouse && (mouse || d.key == key) })
}

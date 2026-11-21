package engine

import (
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// recordingFrameWindow keeps the window each RequestFrame went to so the
// test can drive the engine with exactly the response it would receive.
type recordingFrameWindow struct {
	recorder
	id   uint32
	next chan<- uint32
}

func (w *recordingFrameWindow) RequestFrame() { w.next <- w.id }

func TestFrameDriverFollowsVisibleWindows(t *testing.T) {
	asked := make(chan uint32, 32)
	seen := make(chan struct{}, 32)
	start := make(chan func(), 1)
	app := &recordingFrameWindow{recorder: recorder{mounted: make(chan struct{}, 1)}, next: asked}
	e := Start(app, core.AppRoot(func(cx *core.Scope) {
		view := func(*core.Scope) core.Node { return core.View{} }
		cx.SetMainView(view)
		cx.OpenWindow(core.WindowOptions{}, view)
		cx.OpenWindow(core.WindowOptions{}, view)
		start <- func() {
			cx.Post(func() {
				var tick func(time.Time)
				tick = func(time.Time) {
					seen <- struct{}{}
					cx.RequestAnimationFrame(tick)
				}
				cx.RequestAnimationFrame(tick)
			})
		}
	}), Options{Windows: &core.WindowOptions{Width: 200, Height: 100}})
	defer e.Close()
	app.wait(t, "three windows to open", func() bool {
		return len(app.items(driver.ItemOpenSurface)) == 3
	})
	var ids []uint32
	for _, it := range app.items(driver.ItemOpenSurface) {
		ids = append(ids, it.Surface.ID)
	}
	slices.Sort(ids)
	windows := make(map[uint32]*Engine)
	for i := len(ids) - 1; i >= 0; i-- {
		id := ids[i]
		w := &recordingFrameWindow{id: id, next: asked}
		windows[id] = e.Attach(id, w)
		windows[id].SetFrameWindow(true, false)
	}
	e.SetFrameWindow(false, false)

	expectRequest := func(want uint32) {
		t.Helper()
		e.Sync()
		select {
		case got := <-asked:
			if got != want {
				t.Fatalf("asked window %d, want %d", got, want)
			}
		default:
			t.Fatalf("window %d did not receive a request", want)
		}
		if len(asked) != 0 {
			t.Fatal("a frame was requested from more than one window")
		}
	}
	respond := func(source *Engine, want bool) {
		t.Helper()
		source.Frame()
		e.Sync()
		select {
		case <-seen:
			if !want {
				t.Fatal("a former driver advanced the app's frames")
			}
		default:
			if want {
				t.Fatal("the chosen driver's response did not advance the app's frames")
			}
		}
	}
	low, middle, high := ids[0], ids[1], ids[2]
	(<-start)()
	expectRequest(low)
	for range 12 {
		respond(windows[low], true)
		expectRequest(low)
	}

	// Focus moves the pending request. A late response from the former
	// driver cannot consume that request.
	windows[middle].SetFrameWindow(true, true)
	expectRequest(middle)
	respond(windows[low], false)
	respond(windows[middle], true)
	expectRequest(middle)

	windows[middle].SetFrameWindow(true, false)
	windows[high].SetFrameWindow(true, true)
	expectRequest(high)

	// Minimize the only focused window before it ticks. The window that
	// most recently had focus and can still tick receives the request.
	windows[high].SetFrameWindow(false, false)
	expectRequest(middle)
	respond(windows[high], false)
	respond(windows[middle], true)
	expectRequest(middle)

	windows[middle].Close()
	expectRequest(low)
	respond(windows[middle], false)
	windows[low].SetFrameWindow(false, false)
	expectRequest(0)
	respond(e, true)
	expectRequest(0)
}

func TestUnreportedFrameWindowCanForwardFrame(t *testing.T) {
	asked := make(chan uint32, 1)
	surfaces := map[uint32]*surface{
		0: {p: &recordingFrameWindow{next: asked}},
		1: {id: 1, p: &recordingFrameWindow{id: 1, next: asked}},
	}
	f := frameRequests{wanted: true}
	f.request(surfaces)
	if got := <-asked; got != 1 {
		t.Fatalf("asked surface %d, want the attached scene", got)
	}
	if !f.receive(0, surfaces) || f.wanted {
		t.Fatal("a frame forwarded to the first surface did not answer the request")
	}
}

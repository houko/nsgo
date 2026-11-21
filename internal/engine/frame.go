package engine

type frameWindow uint32

const (
	frameWindowUnknown frameWindow = iota
	frameWindowHidden
	frameWindowVisible
	frameWindowFocused
)

// frameRequests routes the reactive graph's single pending frame to a
// window that can supply it. Only the engine goroutine uses it.
type frameRequests struct {
	wanted     bool
	driver     *surface
	focusOrder uint64
}

func (f *frameRequests) updateWindow(s *surface, window frameWindow) {
	if window == frameWindowFocused && s.frameWindow != frameWindowFocused {
		f.focusOrder++
		s.lastFocused = f.focusOrder
	}
	s.frameWindow = window
}

func (f *frameRequests) request(surfaces map[uint32]*surface) {
	if !f.wanted {
		return
	}
	var best *surface
	for id, s := range surfaces {
		if id == 0 || s.frameWindow == frameWindowHidden {
			continue
		}
		if best == nil || s.lastFocused > best.lastFocused ||
			s.lastFocused == best.lastFocused && s.id < best.id {
			best = s
		}
	}
	if best == nil {
		best = surfaces[0]
	}
	if f.driver != best {
		f.driver = best
		best.p.RequestFrame()
	}
}

// receive ignores a window's late response after a handoff. Backends
// that do not report window state can forward the request between their
// own surfaces, as iOS does from a background scene to a foreground one.
func (f *frameRequests) receive(id uint32, surfaces map[uint32]*surface) bool {
	if !f.wanted || f.driver == nil {
		return false
	}
	if surfaces[f.driver.id] != f.driver {
		return false
	}
	if id != f.driver.id {
		source := surfaces[id]
		if source == nil || source.frameWindow != frameWindowUnknown || f.driver.frameWindow != frameWindowUnknown {
			return false
		}
	}
	if f.driver.frameWindow == frameWindowHidden && f.driver.id != 0 {
		return false
	}
	f.wanted, f.driver = false, nil
	return true
}

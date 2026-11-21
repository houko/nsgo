package driver

import "time"

// DisplayTime returns the time a display frame stamped stamp has on time.Now's base, given now, the platform clock's reading that stamp counts on (CACurrentMediaTime, System.nanoTime, performance.now, g_get_monotonic_time). Both readings are the same platform clock's, taken on the main thread as its frame callback runs, so the result keeps the display's cadence and Go's monotonic reading whatever later delays the frame on its way to the engine (engine.Engine.FrameAt).
func DisplayTime(stamp, now time.Duration) time.Time {
	return time.Now().Add(stamp - now)
}

// DisplayClock brings the stamps of a display whose clock Go cannot read to time.Now's base, as a HarmonyOS web view gets its host's frames over an asynchronous bridge. It keeps the least difference seen between a stamp's arrival and the stamp, the bridge's quickest delivery, so the frames' times keep the display's cadence, never lie ahead of their arrival, and settle on that delivery as quicker ones come.
type DisplayClock struct {
	base   time.Time
	offset time.Duration
}

// At returns the time of the frame stamped stamp, on the display's clock, which arrived at now.
func (c *DisplayClock) At(stamp time.Duration, now time.Time) time.Time {
	if c.base.IsZero() {
		c.base = now
		c.offset = -stamp
	}
	if d := now.Sub(c.base) - stamp; d < c.offset {
		c.offset = d
	}
	return c.base.Add(stamp + c.offset)
}

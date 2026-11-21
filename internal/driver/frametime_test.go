package driver

import (
	"testing"
	"time"
)

// A display's stamp lands as far from now as it is from the platform's reading.
func TestDisplayTimeKeepsTheStampsDistance(t *testing.T) {
	before := time.Now()
	at := DisplayTime(10*time.Second+FrameInterval, 10*time.Second)
	after := time.Now()
	if at.Before(before.Add(FrameInterval)) || at.After(after.Add(FrameInterval)) {
		t.Fatalf("a stamp a frame ahead landed at %v, want a frame after %v to %v", at, before, after)
	}
}

// Frames delivered late keep the display's cadence, and a quicker delivery moves the frames back to it without putting one ahead of its arrival.
func TestDisplayClockKeepsTheDisplaysCadence(t *testing.T) {
	var c DisplayClock
	start := time.Unix(100, 0)
	stamp := func(i int) time.Duration { return time.Hour + time.Duration(i)*FrameInterval }
	delays := []time.Duration{5 * time.Millisecond, 9 * time.Millisecond, 6 * time.Millisecond, 7 * time.Millisecond}
	var last time.Time
	for i, d := range delays {
		arrived := start.Add(time.Duration(i)*FrameInterval + d)
		at := c.At(stamp(i), arrived)
		if at.After(arrived) {
			t.Fatalf("frame %d's time %v lies ahead of its arrival %v", i, at, arrived)
		}
		if i > 0 && at.Sub(last) != FrameInterval {
			t.Fatalf("frame %d came %v after the one before, want the display's %v", i, at.Sub(last), FrameInterval)
		}
		last = at
	}
	// A quicker delivery: the frame lands at its arrival, nearer its stamp.
	arrived := start.Add(4*FrameInterval + time.Millisecond)
	if at := c.At(stamp(4), arrived); !at.Equal(arrived) {
		t.Fatalf("the quickest frame's time = %v, want its arrival %v", at, arrived)
	}
	if at := c.At(stamp(5), arrived.Add(FrameInterval+4*time.Millisecond)); !at.Equal(arrived.Add(FrameInterval)) {
		t.Fatalf("the next frame's time = %v, want a frame after the quickest's", at)
	}
}

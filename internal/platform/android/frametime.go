//go:build android

package android

import (
	"syscall"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// clockMonotonic is CLOCK_MONOTONIC, the clock System.nanoTime and Choreographer's frame times count on.
const clockMonotonic = 1

// frameTime brings a msgFrame's frameTimeNanos, when the Choreographer frame's vsync came, to time.Now's base: read against the same clock here, the pipe's delay drops out. A frame that carries none, or a clock that cannot be read, gives the zero time, the engine's clock.
func frameTime(r *reader) time.Time {
	if len(r.buf) < 8 {
		return time.Time{}
	}
	stamp := time.Duration(r.u64())
	var ts syscall.Timespec
	if _, _, errno := syscall.Syscall(syscall.SYS_CLOCK_GETTIME, clockMonotonic, uintptr(unsafe.Pointer(&ts)), 0); errno != 0 {
		return time.Time{}
	}
	return driver.DisplayTime(stamp, time.Duration(ts.Nano()))
}

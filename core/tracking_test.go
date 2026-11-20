package core

import "testing"

func TestAReadFromADisposedComputationIsNotRecorded(t *testing.T) {
	var failure string
	newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		value := NewSignal(cx, 1)
		c := newComputation(cx.current(), renderEffect, func() bool { return false })
		rt := c.rt

		rt.observer = c
		value.Get()
		rt.observer = nil
		if got := len(value.subs.list); got != 1 {
			failure = "live computation left readers"
			return View{}
		}
		c.Dispose()
		if got := len(value.subs.list); got != 0 {
			failure = "disposing computation left readers"
			return View{}
		}

		rt.observer = c
		value.Get()
		rt.observer = nil
		if got := len(value.subs.list); got != 0 {
			failure = "disposed computation read was recorded"
		}
		return View{}
	})
	if failure != "" {
		t.Fatal(failure)
	}
}

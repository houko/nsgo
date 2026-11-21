//go:build !js

package tsx

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

func TestSandboxStackLimit(t *testing.T) {
	for _, test := range []struct {
		name   string
		limits *Limits
		want   int
	}{
		{name: "default limits", want: DefaultLimits.Stack},
		{name: "zero uses the native default", limits: &Limits{}, want: maxCallStack},
		{name: "negative uses the native default", limits: &Limits{Stack: -1}, want: maxCallStack},
		{name: "lower limit", limits: &Limits{Stack: 32}, want: 32},
		{name: "at the ceiling", limits: &Limits{Stack: maxCallStack}, want: maxCallStack},
		{name: "above the ceiling", limits: &Limits{Stack: maxCallStack + 1}, want: maxCallStack},
		{name: "largest int", limits: &Limits{Stack: math.MaxInt}, want: maxCallStack},
	} {
		t.Run(test.name, func(t *testing.T) {
			cx := core.NewRoot(core.RootOptions{})
			defer cx.Dispose()
			core.NewTree(cx, core.ApproxTextEngine{})

			var original Limits
			if test.limits != nil {
				original = *test.limits
			}
			s := Sandbox{Limits: test.limits}
			var r *runtime
			cx.Run(func() {
				r = s.start(cx, "stack.js", "globalThis.answer = (() => 42)();")
			})
			if r.limits.Stack != test.want {
				t.Errorf("runtime limit = %d, want %d", r.limits.Stack, test.want)
			}
			if test.limits != nil && *test.limits != original {
				t.Errorf("caller limits = %+v, want %+v", *test.limits, original)
			}
			if got := r.vm.Get("answer").ToInteger(); got != 42 {
				t.Errorf("answer = %d, want 42", got)
			}
		})
	}
}

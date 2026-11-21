//go:build !js

package tsx

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx/internal/js"
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

// TestSandboxStackCeilingSurvives has a plugin recurse through native callbacks, a native getter and Reflect.apply with Limits.Stack far above the ceiling, in a process of its own as overflowing the goroutine's stack ends the process: each call must fail at the ceiling with a stack error (#1093).
func TestSandboxStackCeilingSurvives(t *testing.T) {
	if os.Getenv("NSGO_TEST_STACK_CEILING") != "" {
		cx := core.NewRoot(core.RootOptions{})
		defer cx.Dispose()
		core.NewTree(cx, core.ApproxTextEngine{})
		s := Sandbox{Limits: &Limits{Load: time.Minute, Call: time.Minute, Stack: 1_000_000}}
		var r *runtime
		cx.Run(func() { r = s.start(cx, "stack.js", "") })
		for name, src := range map[string]string{
			"map":           `const f = () => [0].map(f); f();`,
			"getter":        `const o = {}; Object.defineProperty(o, "toString", { get: Object.prototype.toLocaleString }); String(o);`,
			"Reflect.apply": `let a = [() => 1, null, []]; for (let i = 0; i < 1e6; i++) a = [Reflect.apply, null, a]; Reflect.apply(...a);`,
		} {
			// A fresh runtime per case, as one that overflowed is not reused.
			if name != "map" {
				cx.Run(func() { r = s.start(cx, "stack.js", "") })
			}
			fn, err := r.vm.RunString(`(function () { let caught; try { ` + src + ` caught = "none"; } catch (e) { caught = String(e); } return caught; })`)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			call, _ := js.AssertFunction(fn)
			// What it fails with, as Plugin.call recovers it.
			got := func() (got string) {
				defer func() {
					if x := recover(); x != nil {
						got = fmt.Sprint(x)
					}
				}()
				v, err := call(js.Undefined())
				if err != nil {
					return err.Error()
				}
				return v.String()
			}()
			if !strings.Contains(got, "Maximum call stack size exceeded") {
				t.Fatalf("%s: %s, want a stack error", name, got)
			}
		}
		return
	}
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestSandboxStackCeilingSurvives$", "-test.v")
	cmd.Env = append(os.Environ(), "NSGO_TEST_STACK_CEILING=1")
	cmd.WaitDelay = time.Second
	timer := time.AfterFunc(2*time.Minute, func() { _ = cmd.Process.Kill() })
	defer timer.Stop()
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := string(out)
		if i := strings.Index(text, "\ngoroutine "); i >= 0 {
			text = text[:i]
		}
		t.Fatalf("%v\n%s", err, text)
	}
}

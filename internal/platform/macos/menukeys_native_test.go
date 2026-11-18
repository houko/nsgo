//go:build darwin && !ios

package macos

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// TestMenuAutoEditingKey presses ⌘Z, as AppKit delivers it, against an app's own Undo item (mod+z, MenuAuto) with no role: with a field focused the field undoes its typing and the item does not run, with a view focused the item runs, and clicked with the field focused the item runs too. Run owns AppKit's process lifetime, so the test gets a process of its own.
func TestMenuAutoEditingKey(t *testing.T) {
	if os.Getenv("NSGO_TEST_MENU_KEY_PROCESS") != t.Name() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_MENU_KEY_PROCESS="+t.Name(), "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	const title = "nsgo-menu-key"
	app := func(cx *core.Scope) core.Node {
		var button, field core.Ref
		focused := core.NewSignal(cx, "ready")
		undone := core.NewSignal(cx, 0)
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%s|%d", title, focused.Get(), undone.Get())) })
		cx.SetApplicationMenu([]core.MenuItem{{Label: "Edit", Items: []core.MenuItem{
			{Label: "Undo Move", Shortcut: "mod+z", When: core.MenuAuto, OnSelect: func() { undone.Update(func(n int) int { return n + 1 }) }},
		}}})
		cx.OnKeyWith("mod+f", core.KeyOptions{When: core.KeyAlways}, field.Focus)
		cx.OnKeyWith("mod+b", core.KeyOptions{When: core.KeyAlways}, button.Focus)
		cx.OnKeyWith("mod+m", core.KeyOptions{When: core.KeyAlways}, func() { focused.Set(focused.Get() + "-checked") })
		cx.OnKeyWith("mod+q", core.KeyOptions{When: core.KeyAlways}, cx.Quit)
		return core.View{Style: core.Style{Width: core.Pt(200), Height: core.Pt(200)}, Children: []core.Node{
			core.Input{TestID: "field", Ref: &field, DefaultValue: "hello", Style: core.Style{Height: core.Pt(30)},
				OnFocus: func() { focused.Set("field") }},
			core.View{TestID: "button", Ref: &button, Style: core.Style{Width: core.Pt(100), Height: core.Pt(40)},
				OnFocus: func() { focused.Set("button") }, OnPress: func(core.PressEvent) {}},
		}}
	}
	done := make(chan error, 1)
	go func() {
		var live *surface
		// main runs fn on the main queue and waits for it.
		main := func(fn func()) {
			barrier := make(chan struct{})
			apple.OnMain(func() {
				defer close(barrier)
				fn()
			})
			<-barrier
		}
		wait := func(want string) error {
			deadline := time.Now().Add(5 * time.Second)
			last := ""
			for time.Now().Before(deadline) {
				main(func() {
					for s := range surfaces {
						if s.window != 0 && strings.HasPrefix(apple.GoString(s.window.Send(sel("title"))), title) {
							live = s
						}
					}
					if live != nil {
						last = apple.GoString(live.window.Send(sel("title")))
					}
				})
				if last == want {
					return nil
				}
				time.Sleep(10 * time.Millisecond)
			}
			return fmt.Errorf("window did not reach %q: it shows %q", want, last)
		}
		binding := func(name string) {
			apple.OnMain(func() {
				if live == nil {
					finish()
					return
				}
				// The platform reports a key binding by its ID, as it does a matched press.
				for _, b := range live.keys {
					if b.Key == name {
						live.engine.Dispatch(driver.Event{Kind: driver.EventKey, Count: b.ID})
					}
				}
			})
		}
		// commandZ queues a ⌘Z key press for the window, which AppKit takes from its queue as it does a typed one: the current event while the menu looks for its key equivalent.
		commandZ := func() {
			main(func() {
				const keyDown, command, keyZ = 10, 1 << 20, 6
				z := apple.NSString("z")
				e := objc.ID(apple.Class("NSEvent")).Send(sel("keyEventWithType:location:modifierFlags:timestamp:windowNumber:context:characters:charactersIgnoringModifiers:isARepeat:keyCode:"),
					uint(keyDown), apple.CGPoint{}, uint(command), float64(0), objc.Send[int](live.window, sel("windowNumber")), objc.ID(0), z, z, false, uint16(keyZ))
				nsApp().Send(sel("postEvent:atStart:"), e, false)
			})
		}
		text := func() (got string) {
			main(func() {
				if e := live.window.Send(sel("firstResponder")); objc.Send[bool](e, sel("isKindOfClass:"), apple.Class("NSText")) {
					got = apple.GoString(e.Send(sel("string")))
				}
			})
			return got
		}
		check := func() error {
			if err := wait(title + "|ready|0"); err != nil {
				return err
			}
			main(func() { live.window.Send(sel("makeKeyAndOrderFront:"), objc.ID(0)) })
			binding("mod+f")
			if err := wait(title + "|field|0"); err != nil {
				return err
			}
			main(func() {
				editor := live.window.Send(sel("firstResponder"))
				editor.Send(sel("setSelectedRange:"), apple.NSRange{Location: uint(len("hello"))})
				editor.Send(sel("insertText:"), apple.NSString(" world"))
			})
			if got := text(); got != "hello world" {
				return fmt.Errorf("the field shows %q after typing, want %q", got, "hello world")
			}
			commandZ()
			got, deadline := text(), time.Now().Add(5*time.Second)
			for got != "hello" && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
				got = text()
			}
			if got != "hello" {
				return fmt.Errorf("⌘Z with the field focused left %q, want the typing undone: %q", got, "hello")
			}
			// ⌘Z has been handled, and the marker's key comes after any report it made, so the count is final once the marker shows.
			binding("mod+m")
			if err := wait(title + "|field-checked|0"); err != nil {
				return fmt.Errorf("⌘Z with the field focused: %w", err)
			}
			// Clicked, the item is the app's even with the field focused. A click is AppKit's current event while the item is chosen; a queued event of the app's own stands for it, so ⌘Z is no longer the current event.
			const appDefined = 15 // NSEventTypeApplicationDefined
			main(func() {
				e := objc.ID(apple.Class("NSEvent")).Send(sel("otherEventWithType:location:modifierFlags:timestamp:windowNumber:context:subtype:data1:data2:"),
					uint(appDefined), apple.CGPoint{}, uint(0), float64(0), objc.Send[int](live.window, sel("windowNumber")), objc.ID(0), int16(0), 0, 0)
				nsApp().Send(sel("postEvent:atStart:"), e, false)
			})
			for current, deadline := uint(0), time.Now().Add(5*time.Second); current != appDefined; time.Sleep(10 * time.Millisecond) {
				if time.Now().After(deadline) {
					return fmt.Errorf("AppKit did not take the queued event")
				}
				main(func() {
					if e := nsApp().Send(sel("currentEvent")); e != 0 {
						current = objc.Send[uint](e, sel("type"))
					}
				})
			}
			main(func() {
				edit := nsApp().Send(sel("mainMenu")).Send(selItemAtIndex, 0).Send(sel("submenu"))
				edit.Send(sel("performActionForItemAtIndex:"), 0)
			})
			if err := wait(title + "|field-checked|1"); err != nil {
				return fmt.Errorf("the item clicked with the field focused: %w", err)
			}
			binding("mod+b")
			if err := wait(title + "|button|1"); err != nil {
				return err
			}
			commandZ()
			if err := wait(title + "|button|2"); err != nil {
				return fmt.Errorf("⌘Z with a view focused: %w", err)
			}
			return nil
		}
		err := check()
		done <- err
		binding("mod+q")
	}()
	onMain(func() {
		Run(core.AppRoot(func(cx *core.Scope) { cx.SetMainView(app) }), Options{Title: title, Width: 200, Height: 200})
	})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

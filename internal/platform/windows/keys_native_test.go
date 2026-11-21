//go:build windows

package windows

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestKeysComeUpAsTheWindowLosesTheKeyboard holds a key on a focused view that hears it, for an app that reads the keys held: a key that goes down again without repeating reports its lost up first, so core.KeysDown does not keep it, the key held comes up as the window loses the keyboard (WM_KILLFOCUS) and leaves core.KeysDown empty, and its WM_KEYUP that comes later is not heard again (#1046).
func TestKeysComeUpAsTheWindowLosesTheKeyboard(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-keys-up"
	app := func(cx *core.Scope) core.Node {
		log := core.NewSignal(cx, "")
		mark := func(prefix string, e core.KeyEvent) {
			s := prefix + e.Key
			if e.Repeat {
				s += "!"
			}
			log.Update(func(l string) string {
				if l != "" {
					l += ","
				}
				return l + s
			})
		}
		cx.Effect(func() {
			cx.SetTitle(fmt.Sprintf("%s|%s|%s|", title, log.Get(), strings.Join(core.KeysDown(cx), " ")))
		})
		return core.View{Style: core.Style{Padding: driver.All(20)}, Children: []core.Node{
			core.View{TestID: "pad", Focusable: func() bool { return true }, Style: core.Style{Width: driver.Pt(100), Height: driver.Pt(40)},
				OnKeyDown: func(e core.KeyEvent) bool { mark("d:", e); return false },
				OnKeyUp:   func(e core.KeyEvent) { mark("u:", e) }},
		}}
	}
	hwnd := startApp(t, title, app, 320, 200)
	waitForTitle(t, hwnd, title+"|||", 5*time.Second)
	onMain(t, hwnd, func(w *window) {
		if !w.heldKeys {
			t.Fatal("reading the keys held did not ask for every key")
		}
		for _, v := range w.views {
			if v != nil && !v.deleted && v.props.TestID == "pad" {
				w.focusView(v)
			}
		}
		if v := w.focusedView(); v == nil || v.props.TestID != "pad" {
			t.Fatal("the pad did not take the focus")
		}
	})
	// key posts the B key's (scan code 0x30) message, with lParam's previous-state bit set for a repeat and an up.
	key := func(message uint32, vk uintptr, scan uintptr, repeat bool) {
		lParam := 1 | scan<<16
		if repeat {
			lParam |= 1 << 30
		}
		if message == wmKeyUp {
			lParam |= 1<<30 | 1<<31
		}
		postMessageW.Call(hwnd, uintptr(message), vk, lParam)
	}
	const vkB, scanB, vkC, scanC = 0x42, 0x30, 0x43, 0x2e
	key(wmKeyDown, vkB, scanB, false)
	key(wmKeyDown, vkB, scanB, true)
	waitForTitle(t, hwnd, title+"|d:b,d:b!|b|", 5*time.Second)
	// Its WM_KEYUP never came: the key going down again without repeating reports it first.
	key(wmKeyDown, vkB, scanB, false)
	waitForTitle(t, hwnd, title+"|d:b,d:b!,u:b,d:b|b|", 5*time.Second)
	postMessageW.Call(hwnd, wmKillFocus, 0, 0)
	waitForTitle(t, hwnd, title+"|d:b,d:b!,u:b,d:b,u:b||", 5*time.Second)
	onMain(t, hwnd, func(w *window) {
		if n := len(w.keysHeld); n != 0 {
			t.Errorf("%d keys still held after the window lost the keyboard", n)
		}
	})
	// The up that comes later is not heard again, and the next key goes down anew.
	postMessageW.Call(hwnd, wmSetFocus, 0, 0)
	key(wmKeyUp, vkB, scanB, false)
	key(wmKeyDown, vkC, scanC, false)
	key(wmKeyUp, vkC, scanC, false)
	waitForTitle(t, hwnd, title+"|d:b,d:b!,u:b,d:b,u:b,d:c,u:c||", 5*time.Second)
}

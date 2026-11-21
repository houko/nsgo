package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base/toast"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestToasts shows toasts read as alerts, runs an action and closes its
// toast, closes one with its close button, and lets one time out.
func TestToasts(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var toaster *Toaster
		undone := false
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			toaster = NewToaster(cx)
			return Toasts{Toaster: toaster, TestID: "toasts", CloseLabel: "Close"}
		})
		toaster.Show(toastOf("saved", ToastSuccess, "Saved", "To your library", "Undo", func() { undone = true }))
		toaster.Show(toastOf("sent", ToastPlain, "Sent", "", "", nil))
		app.Advance(time.Second)
		if v := app.Find("saved"); v.Props.Role != core.RoleAlert || v.Props.Label != "Saved. To your library" {
			t.Fatalf("a toast reads %q as %v", v.Props.Label, v.Props.Role)
		}
		app.Press("saved-action")
		app.Advance(time.Second)
		if !undone || app.Exists("saved") {
			t.Fatal("the action did not run and close its toast")
		}
		app.Press("sent-close")
		app.Advance(time.Second)
		if app.Exists("sent") {
			t.Fatal("the close button left its toast")
		}
		toaster.Show(ToastContent{ID: "brief", Title: "Brief", Duration: 2 * time.Second})
		app.Advance(time.Second)
		app.Find("brief")
		app.Advance(2 * time.Second)
		if app.Exists("brief") {
			t.Fatal("a toast stayed past its duration")
		}
	})
}

func TestToastContentOptions(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var toaster *Toaster
		pressed := false
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			toaster = NewToaster(cx)
			return Toasts{Toaster: toaster, TestID: "toasts", CloseLabel: "Close"}
		})
		toaster.Show(ToastContent{ID: "plain", Title: "Plain", NoClose: true, Duration: -1})
		toaster.Show(ToastContent{ID: "open", Title: "Open", Description: "Finished", Trailing: core.Text{Content: "Ready", TestID: "trailing"}, OnPress: func() { pressed = true }, Duration: -1})
		app.Advance(time.Second)
		if app.Exists("plain-close") {
			t.Fatal("NoClose toast still has a close button")
		}
		if !app.Exists("text:Ready") {
			t.Fatal("trailing toast content is missing")
		}
		if v := app.Find("open"); v.Props.Role != core.RoleButton || v.Props.Label != "Open. Finished" {
			t.Fatalf("pressable toast props = role %v label %q", v.Props.Role, v.Props.Label)
		}
		app.Press("open")
		app.Advance(time.Second)
		if !pressed || app.Exists("open") {
			t.Fatalf("whole-toast press: callback=%v exists=%v", pressed, app.Exists("open"))
		}
	})
}

func TestToastSurfaceStyles(t *testing.T) {
	var plain, styled *Toaster
	var stackFill *core.Signal[core.Color]
	var radius *core.Signal[core.Points]
	stackBackground, itemBackground := core.RGB(0x444466), core.RGB(0x121522)
	itemBorder := core.RGB(0xbd365c)
	app, palette := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		plain, styled = NewToaster(cx), NewToaster(cx)
		stackFill = core.NewSignal(cx, stackBackground)
		radius = core.NewSignal(cx, core.Points(24))
		return core.View{Children: []core.Node{
			Toasts{Toaster: plain, NoClose: true},
			Toasts{Toaster: styled, Placement: ToastBottom, NoClose: true,
				Style:     core.Style{Background: core.RGB(0x334422), BorderWidth: 3, CornerRadius: 8},
				StyleFunc: func() core.Style { return core.Style{Background: stackFill.Get()} }},
		}}
	}, nsgotest.ManualFrames())
	plain.Show(ToastContent{ID: "plain", Title: "Saved", Duration: -1})
	plain.Show(ToastContent{ID: "custom", Title: "Saved", Duration: -1,
		Style: core.Style{Background: itemBackground, BorderColor: itemBorder, CornerRadius: 22, Padding: core.All(24)}})
	styled.Show(ToastContent{ID: "inherited", Title: "Saved", Duration: -1})
	styled.Show(ToastContent{ID: "overridden", Title: "Saved", Duration: -1,
		Style:     core.Style{Background: itemBackground, CornerRadius: 20},
		StyleFunc: func() core.Style { return core.Style{CornerRadius: radius.Get()} }})
	app.Settle()
	if v := app.FindMounted("id:custom"); v.Style.Opacity != 0 || v.Style.Transform.TranslateY != -16 {
		t.Fatalf("custom toast started at opacity %v and translation %v", v.Style.Opacity, v.Style.Transform.TranslateY)
	}
	app.Advance(time.Second)
	for _, want := range []struct {
		id            string
		fill, border  core.Color
		width, radius core.Points
	}{
		{"plain", Alpha(palette.Background, 0.92), palette.Border, 1, RadiusCard},
		{"custom", itemBackground, itemBorder, 1, 22},
		{"inherited", stackBackground, palette.Border, 3, 8},
		{"overridden", itemBackground, palette.Border, 3, 24},
	} {
		s := app.Find(want.id).Style
		if s.Background != want.fill || s.BorderColor != want.border || s.BorderWidth != want.width || s.CornerRadius != want.radius {
			t.Errorf("%s surface: background %x, border %x at %v pt, radius %v", want.id, s.Background, s.BorderColor, s.BorderWidth, s.CornerRadius)
		}
		if s.Transition.Duration != 200*time.Millisecond || s.Opacity != 1 {
			t.Errorf("%s lost its transition or starting phase did not end", want.id)
		}
	}
	if plain, custom := app.Find("plain"), app.Find("custom"); custom.Frame.Size.Height-plain.Frame.Size.Height != 24 {
		t.Fatalf("custom padding made the toast %v tall, want %v", custom.Frame.Size.Height, plain.Frame.Size.Height+24)
	}

	overridden := app.Find("overridden").ID
	newFill := core.RGB(0x416631)
	app.Post(func() {
		stackFill.Set(newFill)
		radius.Set(30)
	})
	app.Settle()
	if got := app.Find("inherited").Style.Background; got != newFill {
		t.Fatalf("stack style did not follow its signal: %x", got)
	}
	if got := app.Find("overridden"); got.Style.CornerRadius != 30 || got.Style.Background != itemBackground || got.ID != overridden {
		t.Fatal("a reactive style lost the per-toast override or rebuilt the toast")
	}
	app.Drag("id:overridden", core.Point{Y: 32}, core.Point{Y: 80})
	if s := app.FindMounted("id:overridden").Style; s.Opacity != 0 || s.Transform.Scale != 0.96 {
		t.Fatalf("the swiped toast did not enter its ending style: opacity %v, scale %v", s.Opacity, s.Transform.Scale)
	}
	app.Advance(time.Second)
	if app.Exists("overridden") || !app.Exists("inherited") {
		t.Fatal("a swipe did not dismiss only the swiped toast")
	}
}

// TestToastReplacementFollowsNewContent keeps a toast's mounted view when its
// content is replaced with the same ID, while updating what it shows and does.
func TestToastReplacementFollowsNewContent(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var toaster *Toaster
		firstAction, nextAction := false, false
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			toaster = NewToaster(cx)
			return Toasts{Toaster: toaster, TestID: "toasts", CloseLabel: "Close"}
		})

		toaster.Show(ToastContent{ID: "same", Title: "First", Description: "old", Leading: core.Text{Content: "first-leading"}, Action: "First action", Duration: -1,
			OnAction: func() { firstAction = true }})
		app.Advance(time.Second)
		oldY := toastY(app, "same")

		toaster.Show(ToastContent{ID: "same", Title: "Second", Description: "new", Leading: core.Text{Content: "second-leading"}, Action: "Second action", Duration: -1,
			OnAction: func() { nextAction = true }})
		app.Advance(time.Second)

		texts := app.Texts()
		if slices.Contains(texts, "First") || slices.Contains(texts, "old") || slices.Contains(texts, "first-leading") || slices.Contains(texts, "First action") ||
			!slices.Contains(texts, "Second") || !slices.Contains(texts, "new") || !slices.Contains(texts, "second-leading") || !slices.Contains(texts, "Second action") {
			t.Fatalf("replacement showed texts %v", texts)
		}
		if got := toastY(app, "same"); got != oldY {
			t.Fatalf("replacement moved toast from %v to %v", oldY, got)
		}
		if !slices.Contains(app.Announced, "Second. new") {
			t.Fatalf("the replacement was not read out: %q", app.Announced)
		}
		app.Press("same-action")
		if !nextAction || firstAction {
			t.Fatalf("replacement action states are first=%v next=%v", firstAction, nextAction)
		}
	})
}

// TestToastKeepsContentAcrossOtherToasts builds a toast's content once while other toasts come and go, so a leading spinner or a focused action button is not built again, and lays out a toast without a leading view or an action with no room for them.
func TestToastKeepsContentAcrossOtherToasts(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var toaster *Toaster
		builds := 0
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			toaster = NewToaster(cx)
			return Toasts{Toaster: toaster, TestID: "toasts", CloseLabel: "Close"}
		})
		toaster.Show(ToastContent{ID: "kept", Title: "Kept", Leading: countedBuild{&builds}, Action: "Undo", Duration: -1})
		toaster.Show(ToastContent{ID: "bare", Title: "Bare", Duration: -1})
		app.Advance(time.Second)
		before := builds
		toaster.Show(ToastContent{ID: "other", Title: "Other", Duration: -1})
		app.Advance(time.Second)
		toaster.Dismiss("other")
		app.Advance(time.Second)
		if builds != before {
			t.Fatalf("other toasts built a toast's leading view %d more times", builds-before)
		}
		if row := app.Find("text:Bare").Parent.Parent; len(row.Children) != 2 {
			t.Fatalf("a toast without a leading view or an action lays out %d views beside its text, want only its close button", len(row.Children)-1)
		}
	})
}

// countedBuild counts its builds.
type countedBuild struct{ n *int }

func (c countedBuild) Render(*core.Scope) core.Node {
	*c.n++
	return core.Text{Content: "counted"}
}

// TestToastFromClosingScope shows a toast from a scope that then goes,
// as a dialog's button does as the dialog closes, and still closes it
// with its close button and lets the next one time out.
func TestToastFromClosingScope(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var toaster *Toaster
		var dialog *core.Scope
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			toaster = NewToaster(cx)
			dialog = cx.Child()
			return Toasts{Toaster: toaster, TestID: "toasts", CloseLabel: "Close"}
		})
		app.Post(func() {
			dialog.Post(func() {
				toaster.Show(ToastContent{ID: "accepted", Title: "Accepted"})
				dialog.Dispose()
			})
		})
		app.Advance(time.Second)
		app.Press("accepted-close")
		app.Advance(time.Second)
		if app.Exists("accepted") {
			t.Fatal("a toast shown from a scope that went did not close")
		}
		toaster.Show(ToastContent{ID: "next", Title: "Next", Duration: time.Second})
		app.Advance(2 * time.Second)
		if app.Exists("next") {
			t.Fatal("the next toast stayed past its duration")
		}
	})
}

// TestToastPlacements places the stack at each edge, swipes toward it by
// default and turns it around with Reverse.
func TestToastPlacements(t *testing.T) {
	for _, c := range []struct {
		p    ToastPlacement
		want toast.SwipeDirection
	}{{ToastTop, toast.SwipeUp}, {ToastBottomEnd, toast.SwipeDown}, {ToastStart, toast.SwipeLeft}, {ToastEnd, toast.SwipeRight}} {
		if got := ToastSwipeEdge.direction(c.p, false); got != c.want {
			t.Fatalf("placement %d swipes %v, want %v", c.p, got, c.want)
		}
	}
	if got := ToastSwipeEdge.direction(ToastStart, true); got != toast.SwipeRight {
		t.Fatalf("the start side swipes %v right to left", got)
	}
	if got := ToastSwipeOff.direction(ToastTop, false); got != toast.SwipeOff {
		t.Fatalf("ToastSwipeOff swipes %v", got)
	}
	each(t, func(t *testing.T, e core.Environment) {
		var toaster *Toaster
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			toaster = NewToaster(cx)
			return Toasts{Toaster: toaster, Placement: ToastEnd, Reverse: true, CloseLabel: "Close"}
		})
		toaster.Show(toastOf("old", ToastPlain, "Old", "", "", nil))
		app.Advance(time.Second)
		toaster.Show(toastOf("new", ToastPlain, "New", "", "", nil))
		app.Advance(time.Second)
		if !(toastY(app, "old") <= toastY(app, "new")) {
			t.Fatal("Reverse left the newest toast above the oldest")
		}
	})
}

// TestToastsExpandedTakePresses opens the stack and closes an older toast
// where it shows, not only the one in front.
func TestToastsExpandedTakePresses(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var toaster *Toaster
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			toaster = NewToaster(cx)
			return Toasts{Toaster: toaster, Placement: ToastBottom, TestID: "toasts", CloseLabel: "Close"}
		})
		toaster.Show(ToastContent{ID: "old", Title: "Old", Duration: -1})
		app.Advance(time.Second)
		toaster.Show(ToastContent{ID: "new", Title: "New", Duration: -1})
		app.Advance(time.Second)
		app.Hover("toasts")
		app.Advance(time.Second)
		if !(toastY(app, "old") < toastY(app, "new")) {
			t.Fatal("the expanded toasts lie one over the other")
		}
		app.Press("old-close")
		app.Advance(time.Second)
		if app.Exists("old") || !app.Exists("new") {
			t.Fatal("the close button of the older toast did not close it")
		}
	})
}

// TestToastLimit keeps Limit toasts showing and tells a toast's OnClose
// when it has gone, replaced or timed out.
func TestToastLimit(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var toaster *Toaster
		closed := 0
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			toaster = NewToaster(cx)
			toaster.Limit = 1
			return Toasts{Toaster: toaster, CloseLabel: "Close"}
		})
		toaster.Show(toastOf("a", ToastPlain, "A", "", "", nil))
		toaster.Show(toastOf("b", ToastPlain, "B", "", "", nil))
		app.Advance(100 * time.Millisecond)
		if app.Exists("a") || !app.Exists("b") {
			t.Fatal("Limit 1 did not show the newest alone")
		}
		onClose := func() { closed++ }
		toaster.Show(ToastContent{ID: "c", Title: "C", Duration: time.Second, OnClose: onClose})
		toaster.Show(ToastContent{ID: "c", Title: "C again", Duration: time.Second, OnClose: onClose})
		if closed != 1 {
			t.Fatalf("replacing a toast closed %d", closed)
		}
		app.Advance(3 * time.Second)
		if closed != 2 {
			t.Fatalf("OnClose ran %d times once it timed out", closed)
		}
	})
}

// TestToastClosing closes the newest toast: it stays where it shows and
// takes no presses, and the others close up at once, at every line a stack
// grows from.
func TestToastClosing(t *testing.T) {
	for _, c := range []struct {
		name    string
		p       ToastPlacement
		reverse bool
	}{{"top", ToastTop, false}, {"bottom", ToastBottomEnd, false}, {"top reversed", ToastTop, true},
		{"bottom reversed", ToastBottom, true}, {"side", ToastEnd, false}, {"side reversed", ToastStart, true}} {
		t.Run(c.name, func(t *testing.T) {
			var toaster *Toaster
			app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
				toaster = NewToaster(cx)
				return Toasts{Toaster: toaster, Placement: c.p, Reverse: c.reverse, TestID: "toasts", CloseLabel: "Close"}
			})
			toaster.Show(ToastContent{ID: "old", Title: "Old", Duration: -1})
			app.Advance(time.Second)
			toaster.Show(ToastContent{ID: "new", Title: "New", Action: "Undo", Duration: -1})
			app.Advance(time.Second)
			if app.Find("new").Animated.Set != 0 {
				t.Fatal("the toast's root binds animated values, which backends draw without its Transition")
			}
			height := app.Find("toasts").Frame.Size.Height
			y := toastY(app, "new")
			toaster.Dismiss("new")
			app.Advance(16 * time.Millisecond)
			if !app.Exists("new") {
				t.Fatal("the toast left before its exit")
			}
			if got := toastY(app, "new"); got != y {
				t.Fatalf("the toast going moved from %v to %v", y, got)
			}
			if got := app.Find("toasts").Frame.Size.Height; got >= height {
				t.Fatalf("the stack is %v tall going, %v before: the toast going takes room", got, height)
			}
			settled := toastY(app, "old")
			going := app.Find("new")
			if top := going.Children[len(going.Children)-1]; top.Frame.Size.Width < going.Frame.Size.Width-2 {
				t.Fatalf("nothing covers the toast going, whose buttons would take presses: %v", top.Frame)
			}
			app.Advance(time.Second)
			if app.Exists("new") {
				t.Fatal("the toast stayed after its exit")
			}
			if got := toastY(app, "old"); got != settled {
				t.Fatalf("the older toast's place went from %v to %v once the newest had gone", settled, got)
			}
		})
	}
}

// toastY is where the stack lays a toast out, in the window.
func toastY(app *nsgotest.App, id string) core.Points {
	var y core.Points
	for v := app.Find(id).Parent; v != nil; v = v.Parent {
		y += v.Frame.Origin.Y
	}
	return y
}

// toastOf is a toast's content, short.
func toastOf(id string, kind ToastType, title, description, action string, onAction func()) ToastContent {
	return ToastContent{ID: id, Type: kind, Title: title, Description: description, Action: action, OnAction: onAction}
}

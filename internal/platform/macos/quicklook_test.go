//go:build darwin && !ios

package macos

import (
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// QuickLookUI loads with AppKit, so a window of the app's own reports Quick Look and a view a host embeds, whose app delegate is not NSGo's, does not. The data source answers the request's files as file URLs, a path with spaces intact, and nothing past the end.
func TestQuickLookSource(t *testing.T) {
	onMain(func() {
		loadAppKit()
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))
		if !quickLookAvailable() {
			t.Fatal("QuickLookUI did not load")
		}
		window := objc.ID(classWindow).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
			apple.Rect(0, 0, 160, 100), uint(styleTitled|styleClosable), uint(2), false)
		window.Send(sel("setReleasedWhenClosed:"), false)
		s := newSurface(Options{Width: 160, Height: 100}, false)
		s.window, s.id = window, 1
		window.Send(sel("setContentView:"), s.native)
		defer apple.OnMain(func() {
			delete(surfaces, s)
			delete(views, s.native)
			s.native.Send(selRelease)
			window.Send(sel("close"))
			window.Send(selRelease)
		})
		if !s.environment().CanQuickLook {
			t.Error("a window of the app's own reports no Quick Look")
		}
		s.embedded = true
		if s.environment().CanQuickLook {
			t.Error("a view a host embeds reports Quick Look, whose panel would ask the host's app delegate for files")
		}
		s.embedded = false

		saved, savedIndex := quickLookURLs, quickLookIndex
		if saved != 0 {
			saved.Send(sel("retain"))
		}
		defer func() {
			if quickLookURLs != 0 {
				quickLookURLs.Send(selRelease)
			}
			quickLookURLs, quickLookIndex = saved, savedIndex
		}()
		paths := []string{"/tmp/a file with spaces.txt", "/tmp/second.txt"}
		setQuickLookFiles(&driver.QuickLookRequest{Paths: paths, Index: 7})
		if quickLookIndex != 1 {
			t.Errorf("index = %d; want the last file's", quickLookIndex)
		}
		if n := objc.Send[int](quickLookSource, sel("numberOfPreviewItemsInPreviewPanel:"), objc.ID(0)); n != len(paths) {
			t.Fatalf("the source answers %d items; want %d", n, len(paths))
		}
		for i, want := range paths {
			url := quickLookSource.Send(sel("previewPanel:previewItemAtIndex:"), objc.ID(0), i)
			if !objc.Send[bool](url, sel("isFileURL")) || apple.GoString(url.Send(sel("path"))) != want {
				t.Errorf("item %d = %q; want the file URL of %q", i, apple.GoString(url.Send(sel("absoluteString"))), want)
			}
		}
		if quickLookSource.Send(sel("previewPanel:previewItemAtIndex:"), objc.ID(0), len(paths)) != 0 {
			t.Error("the source answers an item past the end")
		}
	})
}

//go:build darwin && !ios

package macos

import (
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Quick Look previews files in QLPreviewPanel, which takes its items from the controller it finds up the responder chain: the app delegate accepts control (quickLookControl) and hands the panel quickLookSource, which answers the files of the last request as NSURLs, which Quick Look previews themselves.
var (
	// quickLookSource is the panel's data source, made once QuickLookUI loads; zero where it does not, where QuickLook is unavailable.
	quickLookSource objc.ID
	// quickLookURLs is the retained NSArray of the files the panel shows, zero before the first request; quickLookIndex is the one it starts at.
	quickLookURLs  objc.ID
	quickLookIndex int
)

// registerQuickLook loads QuickLookUI, which AppKit does not, and makes the panel's data source.
func registerQuickLook() {
	if quickLookSource != 0 {
		return
	}
	if _, err := purego.Dlopen("/System/Library/Frameworks/Quartz.framework/Frameworks/QuickLookUI.framework/QuickLookUI", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil || apple.Class("QLPreviewPanel") == 0 {
		return
	}
	class := register("NsgoQuickLookSource", apple.Class("NSObject"), []*objc.Protocol{objc.GetProtocol("QLPreviewPanelDataSource")}, []objc.MethodDef{{
		Cmd: sel("numberOfPreviewItemsInPreviewPanel:"),
		Fn: func(objc.ID, objc.SEL, objc.ID) int {
			if quickLookURLs == 0 {
				return 0
			}
			return int(objc.Send[uint](quickLookURLs, sel("count")))
		},
	}, {
		Cmd: sel("previewPanel:previewItemAtIndex:"),
		Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID, index int) objc.ID {
			if quickLookURLs == 0 || index < 0 || index >= int(objc.Send[uint](quickLookURLs, sel("count"))) {
				return 0
			}
			return quickLookURLs.Send(sel("objectAtIndex:"), uint(index))
		},
	}})
	quickLookSource = objc.ID(class).Send(selNew)
}

// quickLookAvailable reports whether QuickLook can show the panel.
func quickLookAvailable() bool { return quickLookSource != 0 }

// quickLookControl are the app delegate's methods that make it the panel's controller while the app has files for it, as Finder's window is.
func quickLookControl() []objc.MethodDef {
	return []objc.MethodDef{{
		Cmd: sel("acceptsPreviewPanelControl:"),
		Fn:  func(objc.ID, objc.SEL, objc.ID) bool { return quickLookSource != 0 && quickLookURLs != 0 },
	}, {
		Cmd: sel("beginPreviewPanelControl:"),
		Fn: func(_ objc.ID, _ objc.SEL, panel objc.ID) {
			panel.Send(sel("setDataSource:"), quickLookSource)
			panel.Send(sel("setCurrentPreviewItemIndex:"), quickLookIndex)
		},
	}, {
		Cmd: sel("endPreviewPanelControl:"),
		Fn: func(_ objc.ID, _ objc.SEL, panel objc.ID) {
			panel.Send(sel("setDataSource:"), objc.ID(0))
		},
	}}
}

// quickLook shows r's files in the preview panel, starting at its index: it opens the panel, or shows the new files in it while it is open, as Finder does when its selection changes.
func quickLook(r *driver.QuickLookRequest) {
	if quickLookSource == 0 || r == nil || len(r.Paths) == 0 {
		return
	}
	urls := objc.ID(apple.Class("NSMutableArray")).Send(selNew)
	for _, path := range r.Paths {
		urls.Send(sel("addObject:"), objc.ID(apple.Class("NSURL")).Send(sel("fileURLWithPath:"), apple.NSString(path)))
	}
	if quickLookURLs != 0 {
		quickLookURLs.Send(selRelease)
	}
	quickLookURLs, quickLookIndex = urls, min(max(r.Index, 0), len(r.Paths)-1)
	panel := objc.ID(apple.Class("QLPreviewPanel")).Send(sel("sharedPreviewPanel"))
	if objc.Send[bool](panel, sel("isVisible")) && panel.Send(sel("dataSource")) == quickLookSource {
		panel.Send(sel("reloadData"))
		panel.Send(sel("setCurrentPreviewItemIndex:"), quickLookIndex)
		return
	}
	panel.Send(sel("makeKeyAndOrderFront:"), objc.ID(0))
}

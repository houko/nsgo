//go:build darwin && !ios

package macos

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

const (
	quitEventClass = uint32(0x61657674) // 'aevt', kCoreEventClass
	quitEventID    = uint32(0x71756974) // 'quit', kAEQuitApplication
	quitReasonKey  = uint32(0x7768793f) // 'why?', keyAEQuitReason
	errorNumberKey = uint32(0x6572726e) // 'errn', keyErrorNumber
)

// forcedQuitEvents holds the Apple event replies that Run sends after shutdown. Only the main thread uses it.
var forcedQuitEvents []uintptr

func isForcedQuitEvent(event objc.ID) bool {
	if event == 0 || objc.Send[uint32](event, sel("eventClass")) != quitEventClass || objc.Send[uint32](event, sel("eventID")) != quitEventID {
		return false
	}
	reason := event.Send(sel("paramDescriptorForKeyword:"), quitReasonKey)
	if reason == 0 {
		reason = event.Send(sel("attributeDescriptorForKeyword:"), quitReasonKey)
	}
	code := objc.Send[uint32](reason, sel("enumCodeValue"))
	if code == 0 {
		code = objc.Send[uint32](reason, sel("typeCodeValue"))
	}
	switch code {
	case 0x6c6f676f, // 'logo', kAELogOut
		0x726c676f, // 'rlgo', kAEReallyLogOut
		0x72727374, // 'rrst', kAEShowRestartDialog
		0x72657374, // 'rest', kAERestart
		0x7273646e, // 'rsdn', kAEShowShutdownDialog
		0x73687574: // 'shut', kAEShutDown
		return true
	}
	return false
}

func suspendForcedQuit() bool {
	manager := objc.ID(apple.Class("NSAppleEventManager")).Send(sel("sharedAppleEventManager"))
	if !isForcedQuitEvent(manager.Send(sel("currentAppleEvent"))) {
		return false
	}
	suspension := objc.Send[uintptr](manager, sel("suspendCurrentAppleEvent"))
	if suspension == 0 {
		return false
	}
	forcedQuitEvents = append(forcedQuitEvents, suspension)
	return true
}

// replyToForcedQuits acknowledges the system's quit after OnQuit and the storage flush. Holding the Apple event reply lets Run stop the event loop and return, while AppKit's own termination replies would cancel the Apple event or exit the process.
func replyToForcedQuits() {
	if len(forcedQuitEvents) == 0 {
		return
	}
	manager := objc.ID(apple.Class("NSAppleEventManager")).Send(sel("sharedAppleEventManager"))
	for _, suspension := range forcedQuitEvents {
		reply := manager.Send(sel("replyAppleEventForSuspensionID:"), suspension)
		const nullDescriptor, errorStringKey = uint32(0x6e756c6c), uint32(0x65727273) // 'null', 'errs'
		if reply != 0 && objc.Send[uint32](reply, sel("descriptorType")) != nullDescriptor {
			reply.Send(sel("removeParamDescriptorWithKeyword:"), errorNumberKey)
			reply.Send(sel("removeParamDescriptorWithKeyword:"), errorStringKey)
		}
		manager.Send(sel("resumeWithSuspensionID:"), suspension)
	}
	forcedQuitEvents = nil
}

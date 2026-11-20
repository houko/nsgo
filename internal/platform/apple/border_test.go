//go:build darwin

package apple

import (
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The from value must outlive the set that replaces and releases the layer's own (#995).
func TestAnimateKeepsTheFromValueAliveAcrossSet(t *testing.T) {
	var retainCount func(uintptr) int
	purego.RegisterLibFunc(&retainCount, load("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"), "CFGetRetainCount")
	layer := objc.ID(objc.GetClass("CAShapeLayer")).Send(selNew)
	defer layer.Send(selRelease)
	path := cgPath(driver.RoundedRect(nil, 0, 0, 10, 10, driver.Corners{}, false, false))
	layer.Send(sel("setPath:"), path)
	Release(path)
	old := uintptr(layer.Send(sel("path")))
	next := cgPath(driver.RoundedRect(nil, 0, 0, 20, 20, driver.Corners{}, false, false))
	defer Release(next)
	tr := driver.Transition{Duration: time.Second}
	var held int
	animate(layer, "path", &tr, false, func() {
		held = retainCount(old)
		layer.Send(sel("setPath:"), next)
	})
	if held < 2 {
		t.Fatalf("the from path had %d references before set replaced it, so set freed it", held)
	}
	anim := layer.Send(sel("animationForKey:"), NSString("nsgo.path"))
	if anim == 0 || anim.Send(sel("fromValue")) != objc.ID(old) {
		t.Fatal("the path animation does not start from the old path")
	}
}

func TestMoveBordersAboveBackdropLayer(t *testing.T) {
	layer := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	backdrop := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	defer layer.Send(selRelease)
	defer backdrop.Send(selRelease)
	layer.Send(sel("addSublayer:"), backdrop)
	setBorders(layer, &driver.ViewStyle{Borders: driver.Edges{Top: 1}, BorderColors: driver.EdgeColors{Top: driver.RGB(0xff0000)}}, &driver.Transition{})
	r := rings[layer]
	if r == nil || r.sides[0] == 0 {
		t.Fatal("per-side border did not create a ring")
	}
	MoveBorders(layer, backdrop)
	if r.sides[0].Send(sel("superlayer")) != backdrop {
		t.Fatal("border remained under the backdrop")
	}
	MoveBorders(layer, 0)
	if r.sides[0].Send(sel("superlayer")) != layer {
		t.Fatal("border did not return to the view layer")
	}
	setBorders(layer, &driver.ViewStyle{}, &driver.Transition{})
}

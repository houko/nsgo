//go:build js && wasm

package web

import (
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

// TestCanvasImageKeepsItsInlineData holds a cached canvas image's inline bytes for as long as it is cached: its key names them by their address, so a collected buffer's address could come back in a new buffer of the same length and be handed the old image (#738).
func TestCanvasImageKeepsItsInlineData(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("run with nsgo test -target web")
	}
	s := &surface{doc: doc}
	v := &view{id: 1}
	src := driver.ImageSource{Data: canvastest.RedPNG()}
	key := src.Key()
	s.loadImage(src, v)
	im := s.canvasImages[key]
	if im == nil {
		t.Fatal("the image was not cached under its key")
	}
	if len(im.src.Data) != len(src.Data) || &im.src.Data[0] != &src.Data[0] {
		t.Fatal("the cached image does not hold its source's bytes, so their address can be reused while the key is cached")
	}
	if s.loadImage(driver.ImageSource{Data: src.Data}, v) != im {
		t.Fatal("the same bytes loaded the image again")
	}
}

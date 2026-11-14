//go:build linux && !android

package linux

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// trayPNG is a w by h PNG of half-transparent red.
func trayPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 128})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestTrayIconIsStraightARGB keeps the icon's colors under partial alpha and its aspect as it scales down.
func TestTrayIconIsStraightARGB(t *testing.T) {
	pix, w, h := trayIcon(trayPNG(t, 128, 64))
	if w != 64 || h != 32 || len(pix) != w*h*4 {
		t.Fatalf("%dx%d, %d bytes", w, h, len(pix))
	}
	if got := pix[:4]; !bytes.Equal(got, []byte{128, 255, 0, 0}) {
		t.Errorf("first pixel ARGB %v, want [128 255 0 0]", got)
	}
	if pix, _, _ := trayIcon([]byte("not a png")); pix != nil {
		t.Error("an invalid PNG gave pixels")
	}
}

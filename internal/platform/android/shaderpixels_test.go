package android

import (
	"image"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestShaderPixelsStayWithinTheBatchBudget(t *testing.T) {
	// Three images of 12 MiB: two fit in a batch's 32 MiB of pixels, the third waits.
	img := func() *image.RGBA { return image.NewRGBA(image.Rect(0, 0, 2048, 1536)) }
	size := 12 << 20
	a, b, c, old := img(), img(), img(), img()
	images := []driver.ImageSource{{Pixels: a, Version: 2}, {Pixels: b, Version: 2}, {Pixels: c, Version: 2}}
	sent := []*image.RGBA{nil, nil, old}

	fresh, held, left := shaderPixels(images, sent, maxShaderPixels)
	if !fresh[0] || !fresh[1] || fresh[2] {
		t.Fatalf("fresh %v, want the first two", fresh)
	}
	if held[0] != a || held[1] != b || held[2] != old {
		t.Fatal("a copy left out of the batch is recorded as sent")
	}
	if left != maxShaderPixels-2*size {
		t.Fatalf("budget left %d, want %d", left, maxShaderPixels-2*size)
	}

	// The next batch sends only the copy Java lacks.
	fresh, held, left = shaderPixels(images, held, maxShaderPixels)
	if fresh[0] || fresh[1] || !fresh[2] || held[2] != c || left != maxShaderPixels-size {
		t.Fatalf("next batch: fresh %v, left %d", fresh, left)
	}

	// Another shader item in a batch draws on what the items before it left.
	fresh, held, _ = shaderPixels(images[:1], nil, size-1)
	if fresh[0] || held[0] != nil {
		t.Fatal("an item over the budget the batch has left was sent")
	}
}

func TestShaderPixelsOverTheLimitAreNotBudgeted(t *testing.T) {
	// Over the limit, writeShader leaves the image out whatever the budget; it costs none of it.
	huge := &image.RGBA{Rect: image.Rect(0, 0, 4096, 4097)}
	small := image.NewRGBA(image.Rect(0, 0, 2, 2))
	images := []driver.ImageSource{{Pixels: huge, Version: 1}, {Pixels: small, Version: 1}}
	fresh, held, left := shaderPixels(images, nil, maxShaderPixels)
	if !fresh[0] || !fresh[1] || held[0] != huge || left != maxShaderPixels-16 {
		t.Fatalf("fresh %v, left %d", fresh, left)
	}
}

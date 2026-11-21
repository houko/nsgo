//go:build darwin

package apple

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shader"
)

// TestShadersReadImages draws a shader that reads an image into a texture
// with Metal and reads the pixels back: the image's left half red, its
// right half blue, stretched over the target and tinted by a uniform.
func TestShadersReadImages(t *testing.T) {
	if !loadMetal() {
		t.Skip("no Metal device")
	}
	ir, err := shader.CompileFragment([]byte(`package main

var Alpha float

func Fragment(dst vec4, pos vec2) vec4 {
	return image0At(pos*image0Size()/rectSize()) * Alpha
}
`))
	if err != nil {
		t.Fatal(err)
	}
	// A 4 by 2 image: red on the left, blue on the right.
	src := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for x := range 4 {
		for y := range 2 {
			c := color.RGBA{255, 0, 0, 255}
			if x >= 2 {
				c = color.RGBA{0, 0, 255, 255}
			}
			src.SetRGBA(x, y, c)
		}
	}
	var encoded bytes.Buffer
	png.Encode(&encoded, src)
	d, err := decode(context.Background(), encoded.Bytes(), request{ratio: 1, scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	tex := textureOf(d.image, 4, 2)
	if tex == 0 {
		t.Fatal("no texture")
	}
	defer tex.Send(selRelease)
	const w, h = 8, 4
	lanes := []float64{w, h, 1, 4, 2, 0.5}
	pix := drawMetal(t, ir, lanes, []objc.ID{tex}, w, h)
	// BGRA, premultiplied, at half alpha.
	left, right := pix[(1*w+1)*4:], pix[(1*w+6)*4:]
	if left[2] < 120 || left[0] > 10 || left[3] < 120 || left[3] > 135 {
		t.Errorf("left pixel %v, want half red", left[:4])
	}
	if right[0] < 120 || right[2] > 10 {
		t.Errorf("right pixel %v, want half blue", right[:4])
	}
}

// TestShaderPixelsUpdateTheirTexture gives a shader layer an image of pixels at three versions: a new copy at the same size goes into the texture the image had, the same copy uploads nothing, and the texture reads as the latest copy.
func TestShaderPixelsUpdateTheirTexture(t *testing.T) {
	if !loadMetal() {
		t.Skip("no Metal device")
	}
	l := NewShaderLayer(1)
	defer l.Release()
	frame := func(c color.RGBA) *image.RGBA {
		px := image.NewRGBA(image.Rect(0, 0, 4, 2))
		for i := 0; i < len(px.Pix); i += 4 {
			px.Pix[i], px.Pix[i+1], px.Pix[i+2], px.Pix[i+3] = c.R, c.G, c.B, c.A
		}
		return px
	}
	pending := func() map[objc.ID]*image.RGBA {
		l.render.mu.Lock()
		defer l.render.mu.Unlock()
		out := l.render.uploads
		l.render.uploads = nil
		for tex := range out {
			tex.Send(selRelease)
		}
		return out
	}
	red := frame(color.RGBA{255, 0, 0, 255})
	l.setImages([]driver.ImageSource{{Pixels: red, Version: 1}})
	tex := l.textures[0]
	if tex == 0 || l.sizes[0] != [2]float64{4, 2} {
		t.Fatalf("texture %v of size %v, want one of 4 by 2", tex, l.sizes[0])
	}
	if up := pending(); len(up) != 1 || up[tex] != red {
		t.Fatalf("uploads = %v, want the red copy into the texture", up)
	}
	l.setImages([]driver.ImageSource{{Pixels: red, Version: 1}})
	if up := pending(); len(up) != 0 {
		t.Fatalf("the same copy uploaded %v", up)
	}
	blue := frame(color.RGBA{0, 0, 255, 255})
	l.setImages([]driver.ImageSource{{Pixels: blue, Version: 2}})
	if l.textures[0] != tex {
		t.Fatal("a new version at the same size made a new texture")
	}
	if up := pending(); len(up) != 1 || up[tex] != blue {
		t.Fatalf("uploads = %v, want the blue copy into the same texture", up)
	}
	writePixels(tex, blue)
	ir, err := shader.CompileFragment([]byte(`package main

func Fragment(dst vec4, pos vec2) vec4 {
	return image0At(pos*image0Size()/rectSize())
}
`))
	if err != nil {
		t.Fatal(err)
	}
	pix := drawMetal(t, ir, []float64{4, 2, 1, 4, 2}, []objc.ID{tex}, 4, 2)
	// BGRA.
	if p := pix[(1*4+1)*4:]; p[0] < 250 || p[2] > 5 || p[3] < 250 {
		t.Errorf("pixel %v, want blue", p[:4])
	}
	wide := image.NewRGBA(image.Rect(0, 0, 8, 2))
	l.setImages([]driver.ImageSource{{Pixels: wide, Version: 3}})
	if l.textures[0] == tex || l.sizes[0] != [2]float64{8, 2} {
		t.Fatalf("a new size kept texture %v of size %v", l.textures[0], l.sizes[0])
	}
	pending()
}

//go:build darwin && !nsgo_noshader

package apple

import (
	"encoding/binary"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestMetalGPUTextureAndDepth(t *testing.T) {
	program, err := core.CompileGPUKage(`
var Light vec4

func Vertex(p vec3, uv vec2) (vec4, vec2) {
	return vec4(p, 1), uv
}

func Fragment(position vec4, uv vec2) vec4 {
	return Texture0At(uv) * Light
}
`)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan driver.Event, 16)
	var layer *GPULayer
	withPool(func() {
		layer = NewGPULayer(driver.GPUView{Generation: 1}, func(kind driver.EventKind, data driver.GPUEvent) {
			events <- driver.Event{Kind: kind, Data: data}
		})
	})
	if layer == nil {
		t.Skip("Metal unavailable")
	}
	t.Cleanup(func() {
		layer.Release()
		select {
		case <-layer.render.done:
			if len(layer.render.resources) != 0 {
				t.Error("GPU resources survived layer release")
			}
		case <-time.After(10 * time.Second):
			t.Error("GPU render queue did not stop")
		}
	})
	var vertices, indices, light []byte
	for _, v := range []float32{
		-1, -1, .2, .25, .5, 1, -1, .2, .25, .5, 0, 1, .2, .25, .5,
		-1, -1, .8, .75, .5, 1, -1, .8, .75, .5, 0, 1, .8, .75, .5,
	} {
		vertices = binary.LittleEndian.AppendUint32(vertices, math.Float32bits(v))
	}
	for _, i := range []uint16{0, 1, 2, 3, 4, 5} {
		indices = binary.LittleEndian.AppendUint16(indices, i)
	}
	for _, v := range []float32{.5, 1, 1, 1} {
		light = binary.LittleEndian.AppendUint32(light, math.Float32bits(v))
	}
	const rgba = driver.GPUFormatRGBA8Unorm
	const depth = driver.GPUFormatDepth32Float
	layer.Submit([]driver.GPUCommand{
		{Op: driver.GPUCreateBuffer, ID: 1, Size: uint64(len(vertices)), Bytes: vertices},
		{Op: driver.GPUCreateBuffer, ID: 2, Size: uint64(len(indices)), Bytes: indices},
		{Op: driver.GPUCreateBuffer, ID: 3, Size: 16},
		{Op: driver.GPUWriteBuffer, ID: 3, Bytes: light},
		{Op: driver.GPUCreateTexture, ID: 4, Texture: &driver.GPUTexture{Width: 2, Height: 1, Format: rgba, Usage: driver.GPUTextureSampled}},
		{Op: driver.GPUWriteTexture, ID: 4, Bytes: []byte{200, 0, 0, 255, 0, 200, 0, 255},
			Region: driver.GPURegion{Width: 2, Height: 1, BytesPerRow: 8}},
		{Op: driver.GPUCreateSampler, ID: 5, Sampler: &driver.GPUSampler{}},
		{Op: driver.GPUCreateTexture, ID: 6, Texture: &driver.GPUTexture{Width: 8, Height: 8, Format: rgba, Usage: driver.GPUTextureRenderAttachment}},
		{Op: driver.GPUCreateTexture, ID: 7, Texture: &driver.GPUTexture{Width: 8, Height: 8, Format: depth, Usage: driver.GPUTextureRenderAttachment}},
		{Op: driver.GPUCreatePipeline, ID: 8, Pipeline: &driver.GPUPipeline{
			Program: program, ColorFormat: rgba, DepthFormat: depth, DepthCompare: driver.GPUCompareLess, DepthWrite: true,
			VertexBuffers: []driver.GPUVertexLayout{{Stride: 20, Attributes: []driver.GPUVertexAttribute{
				{Location: 0, Format: driver.GPUVertexFloat32x3}, {Location: 1, Format: driver.GPUVertexFloat32x2, Offset: 12},
			}}},
		}},
		{Op: driver.GPUBeginPass, Pass: &driver.GPUPass{Color: 6, Depth: 7, HasDepth: true, ClearDepth: 1}},
		{Op: driver.GPUSetPipeline, ID: 8},
		{Op: driver.GPUSetVertexBuffer, ID: 1},
		{Op: driver.GPUSetIndexBuffer, ID: 2},
		{Op: driver.GPUSetUniformBuffer, ID: 3},
		{Op: driver.GPUSetTexture, ID: 4},
		{Op: driver.GPUSetSampler, ID: 5},
		{Op: driver.GPUSetViewport, Viewport: [6]float32{0, 0, 8, 8, 0, 1}},
		{Op: driver.GPUSetScissor, Region: driver.GPURegion{Width: 8, Height: 8}},
		{Op: driver.GPUDrawIndexed, Count: 6, Instances: 1},
		{Op: driver.GPUEndPass},
	})
	layer.Submit([]driver.GPUCommand{{
		Op: driver.GPUReadTexture, ID: 6, Count: 1, Region: driver.GPURegion{X: 4, Y: 4, Width: 1, Height: 1},
	}})
	select {
	case event := <-events:
		result := event.Data.(driver.GPUEvent)
		if event.Kind != driver.EventGPUReadback || result.Code != "" {
			t.Fatalf("Metal event %v: %s", event.Kind, result.Message)
		}
		if want := []byte{100, 0, 0, 255}; !slices.Equal(result.Pixels, want) {
			t.Fatalf("pixel = %v, want %v", result.Pixels, want)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("no Metal readback")
	}
}

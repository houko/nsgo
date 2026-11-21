// Command gpu draws a lit, textured relief with 100,000 triangles in one GPU view.
package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gpu"
)

func main() { nsgo.Run(Root, nsgo.WindowSize(640, 480)) }

const shader = `
var Turn vec4

func Vertex(p vec3, uv vec2, light float) (vec4, vec2, float) {
	x := p.x * Turn.x - p.z * Turn.y
	z := p.x * Turn.y + p.z * Turn.x
	return vec4(x, p.y, 0.5 + z*0.35, 1), uv, light
}

func Fragment(position vec4, uv vec2, light float) vec4 {
	return Texture0At(uv) * vec4(light, light, light, 1)
}
`

func Root(*core.Scope) core.Node {
	return meshView{err: func(err gpu.Error) { log.Print(err) }}
}

type meshView struct {
	ready func(*gpu.Device)
	err   func(gpu.Error)
}

func (v meshView) Render(cx *core.Scope) core.Node {
	// Use the scope inside this component so the display callback and the
	// resources stop with the view.
	return gpu.View{
		Style:       core.Style{FlexGrow: 1},
		DepthFormat: gpu.FormatDepth32Float,
		OnError:     v.err,
		OnReady: func(device *gpu.Device) {
			draw, err := setup(cx, device)
			if err != nil {
				if v.err != nil {
					v.err(gpu.Error{Code: gpu.ErrFailed, Message: err.Error()})
				}
				return
			}
			draw(0)
			if v.ready != nil {
				v.ready(device)
			}
			start := cx.Now()
			var frame func(time.Time)
			frame = func(t time.Time) {
				draw(t.Sub(start).Seconds())
				cx.RequestAnimationFrame(frame)
			}
			cx.RequestAnimationFrame(frame)
		},
	}
}

func setup(cx *core.Scope, dev *gpu.Device) (func(float64), error) {
	const columns, rows = 250, 200
	vertices := make([]byte, 0, (columns+1)*(rows+1)*24)
	indices := make([]byte, 0, columns*rows*12)
	for y := range rows + 1 {
		v := float64(y) / rows
		for x := range columns + 1 {
			u := float64(x) / columns
			height := .18 * math.Cos(4*math.Pi*u) * math.Cos(3*math.Pi*v)
			light := .65 + height
			vertices = encode(vertices, float32(u*1.6-.8), float32(.8-v*1.6), float32(height), float32(u*8), float32(v*8), float32(light))
			if x < columns && y < rows {
				i := uint16(y*(columns+1) + x)
				for _, index := range []uint16{i, i + columns + 1, i + 1, i + 1, i + columns + 1, i + columns + 2} {
					indices = binary.LittleEndian.AppendUint16(indices, index)
				}
			}
		}
	}
	positions, err := dev.CreateBuffer(cx, gpu.BufferDesc{Size: uint64(len(vertices)), Usage: gpu.UsageVertex, Data: vertices})
	if err != nil {
		return nil, err
	}
	elements, err := dev.CreateBuffer(cx, gpu.BufferDesc{Size: uint64(len(indices)), Usage: gpu.UsageIndex, Data: indices})
	if err != nil {
		return nil, err
	}
	uniform, err := dev.CreateBuffer(cx, gpu.BufferDesc{Size: 16, Usage: gpu.UsageUniform | gpu.UsageCopyDst})
	if err != nil {
		return nil, err
	}
	texture, err := dev.CreateTexture(cx, gpu.TextureDesc{Width: 2, Height: 2, Format: gpu.FormatRGBA8Unorm, Usage: gpu.TextureSampled | gpu.TextureCopyDst})
	if err != nil {
		return nil, err
	}
	dev.WriteTexture(texture, gpu.TextureRegion{Width: 2, Height: 2}, []byte{
		242, 182, 64, 255, 35, 116, 177, 255, 35, 116, 177, 255, 242, 182, 64, 255,
	}, gpu.DataLayout{BytesPerRow: 8})
	sampler, err := dev.CreateSampler(cx, gpu.SamplerDesc{AddressU: gpu.AddressRepeat, AddressV: gpu.AddressRepeat})
	if err != nil {
		return nil, err
	}
	layout, err := dev.CreateBindGroupLayout(cx, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Kind: gpu.BindingUniformBuffer}, {Binding: 1, Kind: gpu.BindingTexture}, {Binding: 2, Kind: gpu.BindingSampler},
	})
	if err != nil {
		return nil, err
	}
	bindings, err := dev.CreateBindGroup(cx, layout, []gpu.BindGroupEntry{
		{Binding: 0, Buffer: uniform}, {Binding: 1, Texture: texture}, {Binding: 2, Sampler: sampler},
	})
	if err != nil {
		return nil, err
	}
	module, err := gpu.CompileKage(shader)
	if err != nil {
		return nil, fmt.Errorf("compile mesh: %w", err)
	}
	pipe, err := dev.CreatePipeline(cx, gpu.PipelineDesc{
		Vertex: gpu.ShaderStage{Module: module}, Fragment: gpu.ShaderStage{Module: module},
		Layouts: []*gpu.BindGroupLayout{layout},
		VertexBuffers: []gpu.VertexBufferLayout{{Stride: 24, Attributes: []gpu.VertexAttribute{
			{Location: 0, Format: gpu.VertexFloat32x3},
			{Location: 1, Format: gpu.VertexFloat32x2, Offset: 12},
			{Location: 2, Format: gpu.VertexFloat32, Offset: 20},
		}}},
		DepthStencil: &gpu.DepthStencilState{Format: gpu.FormatDepth32Float, DepthCompare: gpu.CompareLess, DepthWrite: true},
	})
	if err != nil {
		return nil, err
	}
	return func(elapsed float64) {
		angle := .6 * math.Sin(elapsed)
		var data [16]byte
		dev.WriteBuffer(uniform, 0, encode(data[:0], float32(math.Cos(angle)), float32(math.Sin(angle)), 0, 0))
		encoder := dev.BeginEncoder()
		pass := encoder.BeginRenderPass(gpu.RenderPassDesc{
			ColorAttachments: []gpu.ColorAttachment{{ClearColor: core.RGB(0x172131)}},
			DepthStencil:     &gpu.DepthStencilAttachment{ClearDepth: 1, DepthStoreOp: gpu.StoreDiscard},
		})
		pass.SetPipeline(pipe)
		pass.SetVertexBuffer(0, positions, 0)
		pass.SetIndexBuffer(elements, gpu.IndexUint16, 0)
		pass.SetBindGroup(0, bindings)
		pass.DrawIndexed(columns*rows*6, 0, 0)
		pass.End()
		dev.Submit(encoder.Finish())
	}, nil
}

func encode(dst []byte, values ...float32) []byte {
	for _, value := range values {
		dst = binary.LittleEndian.AppendUint32(dst, math.Float32bits(value))
	}
	return dst
}

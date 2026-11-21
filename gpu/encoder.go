package gpu

import (
	"math"
	"slices"

	"github.com/nsgo-dev/nsgo/core"
)

type LoadOp uint8

const (
	LoadClear LoadOp = iota
	LoadLoad
	LoadDiscard
)

type StoreOp uint8

const (
	StoreStore StoreOp = iota
	StoreDiscard
)

type IndexFormat uint8

const (
	IndexUint16 IndexFormat = iota
	IndexUint32
)

type ColorAttachment struct {
	Texture    *Texture
	LoadOp     LoadOp
	StoreOp    StoreOp
	ClearColor core.Color
}
type DepthStencilAttachment struct {
	Texture      *Texture
	DepthLoadOp  LoadOp
	DepthStoreOp StoreOp
	ClearDepth   float64
}
type RenderPassDesc struct {
	ColorAttachments []ColorAttachment
	DepthStencil     *DepthStencilAttachment
}

type Encoder struct {
	device    *Device
	active    *RenderPass
	commands  []core.GPUCommand
	resources []*resource
	err       *Error
	finished  bool
}
type CommandBuffer struct {
	used      bool
	device    *Device
	commands  []core.GPUCommand
	resources []*resource
}

func (d *Device) BeginEncoder() *Encoder { return &Encoder{device: d} }

func (e *Encoder) fail(format string, args ...any) {
	if e.err == nil {
		failure := invalid(format, args...)
		e.err = &failure
	}
}

func (e *Encoder) record(command core.GPUCommand, resource *resource) {
	if e.err != nil || e.finished {
		return
	}
	e.commands = append(e.commands, command)
	if resource != nil {
		e.resources = append(e.resources, resource)
	}
}

func (e *Encoder) BeginRenderPass(desc RenderPassDesc) *RenderPass {
	p := &RenderPass{encoder: e, color: e.device.format, width: uint32(e.device.width), height: uint32(e.device.height)}
	if e.finished || e.active != nil || len(desc.ColorAttachments) != 1 {
		e.fail("a render pass needs one color attachment and an idle encoder")
		return p
	}
	e.active = p
	color := desc.ColorAttachments[0]
	info := &core.GPUPass{ClearColor: color.ClearColor, ColorLoad: uint8(color.LoadOp), ColorStore: uint8(color.StoreOp)}
	if color.LoadOp > LoadDiscard || color.StoreOp > StoreDiscard {
		e.fail("invalid color load or store operation")
		return p
	}
	if texture := color.Texture; texture != nil {
		if !texture.live(e.device) || texture.desc.Usage&TextureRenderAttachment == 0 || depthFormat(texture.desc.Format) {
			e.fail("invalid color texture")
			return p
		}
		info.Color, p.color = texture.id, texture.desc.Format
		p.width, p.height = texture.desc.Width, texture.desc.Height
		e.resources = append(e.resources, &texture.resource)
	} else if color.LoadOp == LoadLoad {
		e.fail("the surface drawable has no previous frame to load")
		return p
	}
	if depth := desc.DepthStencil; depth != nil {
		info.HasDepth = true
		info.ClearDepth = depth.ClearDepth
		info.DepthLoad, info.DepthStore = uint8(depth.DepthLoadOp), uint8(depth.DepthStoreOp)
		if depth.ClearDepth < 0 || depth.ClearDepth > 1 || math.IsNaN(depth.ClearDepth) ||
			depth.DepthLoadOp > LoadDiscard || depth.DepthStoreOp > StoreDiscard {
			e.fail("invalid depth load, store or clear value")
			return p
		}
		if texture := depth.Texture; texture != nil {
			if !texture.live(e.device) || texture.desc.Usage&TextureRenderAttachment == 0 ||
				!depthFormat(texture.desc.Format) || texture.desc.Width != p.width || texture.desc.Height != p.height {
				e.fail("depth texture does not match the pass")
				return p
			}
			info.Depth, p.depth = texture.id, texture.desc.Format
			e.resources = append(e.resources, &texture.resource)
		} else if color.Texture != nil || e.device.depth == FormatUndefined {
			e.fail("a default depth attachment needs a surface pass and a depth format")
			return p
		} else {
			p.depth = e.device.depth
		}
	}
	e.record(core.GPUCommand{Op: core.GPUBeginPass, Pass: info}, nil)
	return p
}

func (e *Encoder) Finish() *CommandBuffer {
	if e.finished || e.active != nil {
		e.fail("finish requires an encoder with every pass ended")
	}
	e.finished = true
	if e.err != nil {
		e.device.fail(*e.err)
		return nil
	}
	return &CommandBuffer{device: e.device, commands: e.commands, resources: e.resources}
}

func (d *Device) Submit(buffers ...*CommandBuffer) {
	var commands []core.GPUCommand
	for i, buffer := range buffers {
		if buffer == nil {
			continue
		}
		if buffer.device != d || buffer.used || slices.Contains(buffers[:i], buffer) {
			d.fail(invalid("command buffer is already used or belongs to another device"))
			return
		}
		for _, r := range buffer.resources {
			if !r.live(d) {
				d.fail(invalid("a submitted GPU resource was released"))
				return
			}
		}
		commands = append(commands, buffer.commands...)
	}
	for _, buffer := range buffers {
		if buffer != nil {
			buffer.used = true
			buffer.commands, buffer.resources = nil, nil
		}
	}
	d.connection.Submit(commands)
}

type bufferBinding struct {
	buffer *Buffer
	offset uint64
}

type RenderPass struct {
	encoder       *Encoder
	color, depth  TextureFormat
	width, height uint32
	pipeline      *Pipeline
	bindings      []*BindGroup
	vertices      [8]bufferBinding
	index         bufferBinding
	indexFormat   IndexFormat
}

func (p *RenderPass) record(command core.GPUCommand, resource *resource) {
	if p.encoder.active != p {
		p.encoder.fail("the render pass has ended or could not start")
		return
	}
	p.encoder.record(command, resource)
}

func (p *RenderPass) End() {
	p.record(core.GPUCommand{Op: core.GPUEndPass}, nil)
	if p.encoder.active == p {
		p.encoder.active = nil
	}
}

func (p *RenderPass) SetPipeline(pipeline *Pipeline) {
	if pipeline == nil || !pipeline.live(p.encoder.device) || pipeline.color != p.color || pipeline.depth != p.depth {
		p.encoder.fail("pipeline attachments must match the pass")
		return
	}
	p.pipeline = pipeline
	p.bindings = make([]*BindGroup, len(pipeline.layouts))
	p.record(core.GPUCommand{Op: core.GPUSetPipeline, ID: pipeline.id}, &pipeline.resource)
}

func (p *RenderPass) SetVertexBuffer(slot uint32, buffer *Buffer, offset uint64) {
	if slot >= uint32(len(p.vertices)) || buffer == nil || !buffer.live(p.encoder.device) ||
		buffer.usage&UsageVertex == 0 || offset >= buffer.size || offset%4 != 0 {
		p.encoder.fail("invalid vertex buffer binding")
		return
	}
	p.vertices[slot] = bufferBinding{buffer, offset}
	p.record(core.GPUCommand{Op: core.GPUSetVertexBuffer, ID: buffer.id, Slot: slot, Offset: offset}, &buffer.resource)
}

func (p *RenderPass) SetIndexBuffer(buffer *Buffer, format IndexFormat, offset uint64) {
	if format > IndexUint32 || buffer == nil || !buffer.live(p.encoder.device) || buffer.usage&UsageIndex == 0 ||
		offset >= buffer.size || offset%indexWidth(format) != 0 {
		p.encoder.fail("invalid index buffer binding")
		return
	}
	p.index, p.indexFormat = bufferBinding{buffer, offset}, format
	p.record(core.GPUCommand{Op: core.GPUSetIndexBuffer, ID: buffer.id, Slot: uint32(format), Offset: offset}, &buffer.resource)
}

func indexWidth(format IndexFormat) uint64 { return 2 << format }

func (p *RenderPass) SetBindGroup(slot uint32, group *BindGroup, offsets ...uint32) {
	if p.pipeline == nil || group == nil || group.closed || uint64(slot) >= uint64(len(p.bindings)) ||
		p.pipeline.layouts[slot] != group.layout || group.layout.closed || len(offsets) != 0 {
		p.encoder.fail("bind group layout must match the pipeline slot")
		return
	}
	var bases [4]uint32
	for _, layout := range p.pipeline.layouts[:slot] {
		for _, entry := range layout.entries {
			bases[entry.Kind]++
		}
	}
	for i, entry := range group.layout.entries {
		value := group.entries[i]
		if !p.encoder.device.acceptBinding(entry.Kind, value) {
			p.encoder.fail("a bound resource was released")
			return
		}
		nativeSlot := bases[entry.Kind]
		bases[entry.Kind]++
		switch entry.Kind {
		case BindingUniformBuffer:
			p.record(core.GPUCommand{Op: core.GPUSetUniformBuffer, ID: value.Buffer.id, Offset: value.Offset}, &value.Buffer.resource)
		case BindingTexture:
			p.record(core.GPUCommand{Op: core.GPUSetTexture, ID: value.Texture.id, Slot: nativeSlot}, &value.Texture.resource)
		case BindingSampler:
			p.record(core.GPUCommand{Op: core.GPUSetSampler, ID: value.Sampler.id, Slot: nativeSlot}, &value.Sampler.resource)
		}
	}
	p.bindings[slot] = group
}

func (p *RenderPass) SetViewport(x, y, width, height, minDepth, maxDepth float32) {
	if !(x >= 0 && y >= 0 && width > 0 && height > 0 && minDepth >= 0 && maxDepth <= 1 && minDepth <= maxDepth &&
		float64(x)+float64(width) <= float64(p.width) && float64(y)+float64(height) <= float64(p.height)) {
		p.encoder.fail("invalid viewport")
		return
	}
	p.record(core.GPUCommand{Op: core.GPUSetViewport, Viewport: [6]float32{x, y, width, height, minDepth, maxDepth}}, nil)
}

func (p *RenderPass) SetScissor(x, y, width, height int32) {
	if x < 0 || y < 0 || width <= 0 || height <= 0 || int64(x)+int64(width) > int64(p.width) ||
		int64(y)+int64(height) > int64(p.height) {
		p.encoder.fail("invalid scissor rectangle")
		return
	}
	p.record(core.GPUCommand{Op: core.GPUSetScissor, Region: core.GPURegion{
		X: uint32(x), Y: uint32(y), Width: uint32(width), Height: uint32(height)}}, nil)
}

func (p *RenderPass) Draw(vertexCount, firstVertex uint32) {
	p.DrawInstanced(vertexCount, 1, firstVertex, 0)
}
func (p *RenderPass) DrawIndexed(indexCount, firstIndex uint32, baseVertex int32) {
	p.DrawIndexedInstanced(indexCount, 1, firstIndex, baseVertex, 0)
}

func (p *RenderPass) DrawInstanced(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	p.draw(false, vertexCount, instanceCount, firstVertex, 0, firstInstance)
}
func (p *RenderPass) DrawIndexedInstanced(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	p.draw(true, indexCount, instanceCount, firstIndex, baseVertex, firstInstance)
}

func (p *RenderPass) draw(indexed bool, count, instances, first uint32, base int32, firstInstance uint32) {
	if count == 0 || instances == 0 {
		return
	}
	if p.pipeline == nil {
		p.encoder.fail("draw needs a pipeline")
		return
	}
	for _, group := range p.bindings {
		if group == nil {
			p.encoder.fail("draw needs all bind groups")
			return
		}
	}
	for i, layout := range p.pipeline.vertex {
		binding := p.vertices[i]
		elements := uint64(first) + uint64(count)
		if layout.Instance {
			elements = uint64(firstInstance) + uint64(instances)
		} else if indexed {
			elements = 1
		}
		if binding.buffer == nil || elements > (binding.buffer.size-binding.offset)/uint64(layout.Stride) {
			p.encoder.fail("draw goes beyond vertex buffer %d", i)
			return
		}
	}
	op := core.GPUDrawVertices
	if indexed {
		b := p.index
		if b.buffer == nil || uint64(first)+uint64(count) > (b.buffer.size-b.offset)/indexWidth(p.indexFormat) {
			p.encoder.fail("draw goes beyond the index buffer")
			return
		}
		op = core.GPUDrawIndexed
	}
	p.record(core.GPUCommand{Op: op, Count: count, Instances: instances, First: first, BaseVertex: base, FirstInstance: firstInstance}, nil)
}

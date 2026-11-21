//go:build darwin

package apple

import (
	"fmt"
	"math"
	"runtime"
	"structs"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
	"github.com/nsgo-dev/nsgo/internal/shaderir/msl"
)

// GPUSupported reports Metal availability. Call it on the main thread.
func GPUSupported() bool { return loadMetal() && mtlQueue != 0 }

// GPULayer is a Metal surface whose resources and command encoders belong to its render goroutine.
type GPULayer struct {
	Layer         objc.ID
	Generation    uint64
	render        *gpuRender
	width, height int
	ready         bool
}

type gpuRender struct {
	layer, queue objc.ID
	config       driver.GPUView
	emit         func(driver.EventKind, driver.GPUEvent)
	wake         chan struct{}
	done         chan struct{}
	mu           sync.Mutex
	pending      [][]driver.GPUCommand
	closed       bool
	drawable     atomic.Bool

	resources   map[driver.GPUID]gpuResource
	depth, last objc.ID
}

type gpuResource struct {
	object, depthState objc.ID
	texture            driver.GPUTexture
	pipeline           *driver.GPUPipeline
}

func (r gpuResource) release() {
	r.object.Send(selRelease)
	r.depthState.Send(selRelease)
}

// NewGPULayer creates the surface on the main thread.
func NewGPULayer(config driver.GPUView, emit func(driver.EventKind, driver.GPUEvent)) *GPULayer {
	if !GPUSupported() {
		return nil
	}
	queue := mtlDevice.Send(sel("newCommandQueue"))
	if queue == 0 {
		return nil
	}
	layer := objc.ID(Class("CAMetalLayer")).Send(selNew)
	layer.Send(sel("setDevice:"), mtlDevice)
	layer.Send(sel("setPixelFormat:"), uint(mtlPixelFormatBGRA8Unorm))
	layer.Send(sel("setFramebufferOnly:"), true)
	layer.Send(sel("setOpaque:"), false)
	layer.Send(sel("setColorspace:"), srgb)
	config.ColorFormat = driver.GPUFormatBGRA8Unorm
	render := &gpuRender{layer: layer, queue: queue, config: config, emit: emit,
		wake: make(chan struct{}, 1), done: make(chan struct{}), resources: make(map[driver.GPUID]gpuResource)}
	go render.loop()
	return &GPULayer{Layer: layer, Generation: config.Generation, render: render}
}

// Resize updates the drawable size and reports it before the application submits a frame.
func (l *GPULayer) Resize(w, h, scale float64) {
	width, height := int(math.Round(w*scale)), int(math.Round(h*scale))
	l.Layer.Send(sel("setContentsScale:"), scale)
	if l.ready && l.width == width && l.height == height {
		return
	}
	l.width, l.height = width, height
	l.Layer.Send(sel("setDrawableSize:"), CGSize{Width: float64(width), Height: float64(height)})
	l.render.drawable.Store(width > 0 && height > 0)
	kind := driver.EventGPUResize
	if !l.ready {
		l.ready = true
		kind = driver.EventGPUDeviceReady
	}
	l.render.event(kind, driver.GPUEvent{Width: width, Height: height, Format: l.render.config.ColorFormat})
}

// Submit queues an immutable command batch.
func (l *GPULayer) Submit(commands []driver.GPUCommand) {
	r := l.render
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.pending = append(r.pending, commands)
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Release detaches the layer and lets the render goroutine release Metal resources.
func (l *GPULayer) Release() {
	l.Layer.Send(sel("removeFromSuperlayer"))
	l.Layer = 0
	r := l.render
	r.mu.Lock()
	r.closed = true
	r.pending = nil
	close(r.wake)
	r.mu.Unlock()
}

func (r *gpuRender) event(kind driver.EventKind, event driver.GPUEvent) {
	event.Generation = r.config.Generation
	r.emit(kind, event)
}

func (r *gpuRender) failure(code, message string) {
	r.event(driver.EventGPUError, driver.GPUEvent{Code: code, Message: message})
}

func (r *gpuRender) loop() {
	defer close(r.done)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for range r.wake {
		r.mu.Lock()
		pending := r.pending
		r.pending = nil
		r.mu.Unlock()
		for _, commands := range pending {
			r.mu.Lock()
			closed := r.closed
			r.mu.Unlock()
			if closed {
				break
			}
			pool := poolPush()
			r.run(commands)
			poolPop(pool)
		}
	}
	pool := poolPush()
	defer poolPop(pool)
	r.last.Send(sel("waitUntilCompleted"))
	r.last.Send(selRelease)
	for _, resource := range r.resources {
		resource.release()
	}
	clear(r.resources)
	r.depth.Send(selRelease)
	r.layer.Send(selRelease)
	r.queue.Send(selRelease)
}

func gpuPixelFormat(format driver.GPUFormat) uint {
	switch format {
	case driver.GPUFormatRGBA8Unorm:
		return mtlPixelFormatRGBA8Unorm
	case driver.GPUFormatBGRA8Unorm:
		return mtlPixelFormatBGRA8Unorm
	case driver.GPUFormatRGBA8UnormSRGB:
		return 71
	case driver.GPUFormatRG8Unorm:
		return 30
	case driver.GPUFormatR8Unorm:
		return 10
	case driver.GPUFormatRGBA16Float:
		return 115
	case driver.GPUFormatDepth24Plus, driver.GPUFormatDepth32Float:
		return 252
	default:
		return 0
	}
}

func gpuNewTexture(desc driver.GPUTexture) objc.ID {
	texture := objc.ID(Class("MTLTextureDescriptor")).Send(
		sel("texture2DDescriptorWithPixelFormat:width:height:mipmapped:"),
		gpuPixelFormat(desc.Format), uint(desc.Width), uint(desc.Height), false)
	// TextureSampled maps to shader read, and TextureRenderAttachment to render target.
	var usage uint
	if desc.Usage&driver.GPUTextureSampled != 0 {
		usage |= 1
	}
	if desc.Usage&driver.GPUTextureRenderAttachment != 0 {
		usage |= 4
	}
	texture.Send(sel("setUsage:"), usage)
	return mtlDevice.Send(sel("newTextureWithDescriptor:"), texture)
}

func gpuNewSampler(s *driver.GPUSampler) objc.ID {
	desc := objc.ID(Class("MTLSamplerDescriptor")).Send(selNew)
	defer desc.Send(selRelease)
	desc.Send(sel("setMinFilter:"), uint(s.MinFilter))
	desc.Send(sel("setMagFilter:"), uint(s.MagFilter))
	address := [...]uint{0, 2, 3}
	desc.Send(sel("setSAddressMode:"), address[s.AddressU])
	desc.Send(sel("setTAddressMode:"), address[s.AddressV])
	return mtlDevice.Send(sel("newSamplerStateWithDescriptor:"), desc)
}

func gpuNewPipeline(p *driver.GPUPipeline) (gpuResource, string) {
	var errObj objc.ID
	lib := mtlDevice.Send(sel("newLibraryWithSource:options:error:"), NSString(msl.CompileGPU(p.Program)), objc.ID(0), unsafe.Pointer(&errObj))
	if lib == 0 {
		return gpuResource{}, errorText(errObj)
	}
	defer lib.Send(selRelease)
	vertex := lib.Send(sel("newFunctionWithName:"), NSString(msl.VertexName))
	fragment := lib.Send(sel("newFunctionWithName:"), NSString(msl.FragmentName))
	defer vertex.Send(selRelease)
	defer fragment.Send(selRelease)
	desc := objc.ID(Class("MTLRenderPipelineDescriptor")).Send(selNew)
	defer desc.Send(selRelease)
	desc.Send(sel("setVertexFunction:"), vertex)
	desc.Send(sel("setFragmentFunction:"), fragment)
	desc.Send(selColorAttachments).Send(selObjectAtIndexedSubscript, uint(0)).Send(sel("setPixelFormat:"), gpuPixelFormat(p.ColorFormat))
	desc.Send(sel("setDepthAttachmentPixelFormat:"), gpuPixelFormat(p.DepthFormat))
	layout := objc.ID(Class("MTLVertexDescriptor")).Send(sel("vertexDescriptor"))
	formats := [...]uint{0, 28, 29, 30, 31, 36}
	for i, b := range p.VertexBuffers {
		entry := layout.Send(sel("layouts")).Send(selObjectAtIndexedSubscript, uint(i))
		entry.Send(sel("setStride:"), uint(b.Stride))
		if b.Instance {
			entry.Send(sel("setStepFunction:"), uint(2))
		} else {
			entry.Send(sel("setStepFunction:"), uint(1))
		}
		for _, a := range b.Attributes {
			attr := layout.Send(sel("attributes")).Send(selObjectAtIndexedSubscript, uint(a.Location))
			attr.Send(sel("setBufferIndex:"), uint(i))
			attr.Send(sel("setOffset:"), uint(a.Offset))
			attr.Send(sel("setFormat:"), formats[a.Format])
		}
	}
	if len(p.VertexBuffers) > 0 {
		desc.Send(sel("setVertexDescriptor:"), layout)
	}
	errObj = 0
	state := mtlDevice.Send(sel("newRenderPipelineStateWithDescriptor:error:"), desc, unsafe.Pointer(&errObj))
	if state == 0 {
		return gpuResource{}, errorText(errObj)
	}
	resource := gpuResource{object: state, pipeline: p}
	if p.DepthFormat != driver.GPUFormatUndefined {
		depth := objc.ID(Class("MTLDepthStencilDescriptor")).Send(selNew)
		defer depth.Send(selRelease)
		compare := [...]uint{7, 0, 1, 3, 2, 6, 4, 5}
		depth.Send(sel("setDepthCompareFunction:"), compare[p.DepthCompare])
		depth.Send(sel("setDepthWriteEnabled:"), p.DepthWrite)
		resource.depthState = mtlDevice.Send(sel("newDepthStencilStateWithDescriptor:"), depth)
		if resource.depthState == 0 {
			resource.release()
			return gpuResource{}, "could not create depth stencil state"
		}
	}
	return resource, ""
}

func gpuBytes(buffer objc.ID, length uint64) []byte {
	p := objc.Send[unsafe.Pointer](buffer, sel("contents"))
	return unsafe.Slice((*byte)(p), int(length))
}

func (r *gpuRender) create(c driver.GPUCommand) {
	var res gpuResource
	switch c.Op {
	case driver.GPUCreateBuffer:
		res.object = mtlDevice.Send(sel("newBufferWithLength:options:"), uint(c.Size), uint(0))
		if res.object != 0 && len(c.Bytes) > 0 {
			copy(gpuBytes(res.object, uint64(len(c.Bytes))), c.Bytes)
		}
	case driver.GPUCreateTexture:
		res.texture = *c.Texture
		res.object = gpuNewTexture(res.texture)
	case driver.GPUCreateSampler:
		res.object = gpuNewSampler(c.Sampler)
	case driver.GPUCreatePipeline:
		var message string
		res, message = gpuNewPipeline(c.Pipeline)
		if res.object == 0 {
			r.failure("shaderCompile", message)
			return
		}
	}
	if res.object == 0 {
		r.failure("outOfMemory", "Metal could not allocate a GPU resource")
		return
	}
	r.resources[c.ID] = res
}

type gpuFrame struct {
	buffer, drawable, encoder objc.ID
	width, height             uint
	pipeline                  *driver.GPUPipeline
	indexBuffer               objc.ID
	indexOffset               uint
	indexType                 uint
}

type gpuOrigin struct {
	_       structs.HostLayout
	X, Y, Z uint
}
type gpuViewport struct {
	_                                       structs.HostLayout
	X, Y, Width, Height, MinDepth, MaxDepth float64
}
type gpuScissor struct {
	_                   structs.HostLayout
	X, Y, Width, Height uint
}

func (r *gpuRender) begin(frame *gpuFrame, pass *driver.GPUPass) bool {
	target := r.resources[pass.Color].object
	if pass.Color == 0 {
		if !r.drawable.Load() {
			return false
		}
		if frame.drawable == 0 {
			frame.drawable = r.layer.Send(selNextDrawable)
		}
		target = frame.drawable.Send(sel("texture"))
	}
	if target == 0 {
		return false
	}
	frame.width, frame.height = uint(target.Send(sel("width"))), uint(target.Send(sel("height")))
	desc := objc.ID(Class("MTLRenderPassDescriptor")).Send(sel("renderPassDescriptor"))
	color := desc.Send(selColorAttachments).Send(selObjectAtIndexedSubscript, uint(0))
	load := [...]uint{2, 1, 0}
	store := [...]uint{1, 0}
	color.Send(sel("setTexture:"), target)
	color.Send(sel("setLoadAction:"), load[pass.ColorLoad])
	color.Send(sel("setStoreAction:"), store[pass.ColorStore])
	red, green, blue, alpha := pass.ClearColor.Components()
	color.Send(sel("setClearColor:"), mtlClearColor{R: red, G: green, B: blue, A: alpha})
	if pass.HasDepth {
		depth := r.resources[pass.Depth].object
		if pass.Depth == 0 {
			if r.depth == 0 || uint(r.depth.Send(sel("width"))) != frame.width || uint(r.depth.Send(sel("height"))) != frame.height {
				r.depth.Send(selRelease)
				r.depth = gpuNewTexture(driver.GPUTexture{Width: uint32(frame.width), Height: uint32(frame.height), Format: r.config.DepthFormat, Usage: driver.GPUTextureRenderAttachment})
			}
			depth = r.depth
		}
		if depth == 0 || uint(depth.Send(sel("width"))) != frame.width || uint(depth.Send(sel("height"))) != frame.height {
			return false
		}
		a := desc.Send(sel("depthAttachment"))
		a.Send(sel("setTexture:"), depth)
		a.Send(sel("setLoadAction:"), load[pass.DepthLoad])
		a.Send(sel("setStoreAction:"), store[pass.DepthStore])
		a.Send(sel("setClearDepth:"), pass.ClearDepth)
	}
	if frame.buffer == 0 {
		frame.buffer = r.queue.Send(sel("commandBuffer"))
	}
	frame.encoder = frame.buffer.Send(sel("renderCommandEncoderWithDescriptor:"), desc)
	frame.pipeline, frame.indexBuffer = nil, 0
	return frame.encoder != 0
}

// run processes one submission. The Metal command queue orders GPU reads and writes,
// and ordinary command buffers retain every resource they reference until completion.
func (r *gpuRender) run(commands []driver.GPUCommand) {
	var frame gpuFrame
	for _, c := range commands {
		resource := r.resources[c.ID]
		switch c.Op {
		case driver.GPUCreateBuffer, driver.GPUCreateTexture, driver.GPUCreateSampler, driver.GPUCreatePipeline:
			r.create(c)
		case driver.GPURelease:
			resource.release()
			delete(r.resources, c.ID)
		case driver.GPUWriteBuffer:
			r.writeBuffer(resource.object, c)
		case driver.GPUWriteTexture:
			if resource.object != 0 {
				r.last.Send(sel("waitUntilCompleted"))
				region := c.Region
				resource.object.Send(sel("replaceRegion:mipmapLevel:withBytes:bytesPerRow:"),
					mtlRegion{X: uint(region.X), Y: uint(region.Y), W: uint(region.Width), H: uint(region.Height), D: 1},
					uint(0), unsafe.Pointer(&c.Bytes[0]), uint(region.BytesPerRow))
			}
		case driver.GPUReadTexture:
			r.readTexture(resource, c)
		case driver.GPUBeginPass:
			r.begin(&frame, c.Pass)
		case driver.GPUEndPass:
			frame.encoder.Send(sel("endEncoding"))
			frame.encoder = 0
		default:
			if frame.encoder != 0 && !frame.draw(c, resource) {
				frame.encoder.Send(sel("endEncoding"))
				frame.encoder = 0
				r.failure("failed", "render pass could not bind a Metal resource or used stale dimensions")
			}
		}
	}
	if frame.buffer != 0 {
		if frame.drawable != 0 {
			frame.buffer.Send(sel("presentDrawable:"), frame.drawable)
		}
		r.commit(frame.buffer)
	}
}

func (r *gpuRender) commit(cb objc.ID) {
	cb.Send(sel("retain"))
	completed := objcblock.New(func(_ objc.Block, buffer objc.ID) {
		const commandError = 5
		if buffer.Send(sel("status")) != commandError {
			return
		}
		err := buffer.Send(sel("error"))
		code := "failed"
		const metalOutOfMemory = 8
		if err.Send(sel("code")) == metalOutOfMemory {
			code = "outOfMemory"
		}
		r.failure(code, errorText(err))
	})
	cb.Send(sel("addCompletedHandler:"), completed)
	completed.Release()
	cb.Send(sel("commit"))
	r.last.Send(selRelease)
	r.last = cb
}

func (r *gpuRender) writeBuffer(target objc.ID, c driver.GPUCommand) {
	if target == 0 {
		return
	}
	staging := mtlDevice.Send(sel("newBufferWithBytes:length:options:"), unsafe.Pointer(&c.Bytes[0]), uint(len(c.Bytes)), uint(0))
	if staging == 0 {
		r.failure("outOfMemory", "Metal could not allocate a buffer upload")
		return
	}
	defer staging.Send(selRelease)
	cb := r.queue.Send(sel("commandBuffer"))
	blit := cb.Send(sel("blitCommandEncoder"))
	blit.Send(sel("copyFromBuffer:sourceOffset:toBuffer:destinationOffset:size:"), staging, uint(0), target, uint(c.Offset), uint(len(c.Bytes)))
	blit.Send(sel("endEncoding"))
	r.commit(cb)
}

func (r *gpuRender) readTexture(texture gpuResource, c driver.GPUCommand) {
	result := driver.GPUEvent{Readback: c.Count, Code: "failed", Message: "Metal texture readback failed"}
	defer func() { r.event(driver.EventGPUReadback, result) }()
	if texture.object == 0 {
		return
	}
	region := c.Region
	row := uint(region.Width) * 4
	stride := (row + 255) &^ 255
	size := stride * uint(region.Height)
	buffer := mtlDevice.Send(sel("newBufferWithLength:options:"), size, uint(0))
	if buffer == 0 {
		result.Code = "outOfMemory"
		return
	}
	defer buffer.Send(selRelease)
	cb := r.queue.Send(sel("commandBuffer"))
	blit := cb.Send(sel("blitCommandEncoder"))
	blit.Send(sel("copyFromTexture:sourceSlice:sourceLevel:sourceOrigin:sourceSize:toBuffer:destinationOffset:destinationBytesPerRow:destinationBytesPerImage:"),
		texture.object, uint(0), uint(0), gpuOrigin{X: uint(region.X), Y: uint(region.Y)},
		gpuOrigin{X: uint(region.Width), Y: uint(region.Height), Z: 1}, buffer, uint(0), stride, size)
	blit.Send(sel("endEncoding"))
	cb.Send(sel("commit"))
	cb.Send(sel("waitUntilCompleted"))
	if cb.Send(sel("status")) != 4 {
		result.Message = fmt.Sprintf("Metal texture readback failed: %s", errorText(cb.Send(sel("error"))))
		return
	}
	raw := gpuBytes(buffer, uint64(size))
	result.Pixels = make([]byte, row*uint(region.Height))
	for y := uint(0); y < uint(region.Height); y++ {
		copy(result.Pixels[y*row:(y+1)*row], raw[y*stride:y*stride+row])
	}
	if texture.texture.Format == driver.GPUFormatBGRA8Unorm {
		for i := 0; i < len(result.Pixels); i += 4 {
			result.Pixels[i], result.Pixels[i+2] = result.Pixels[i+2], result.Pixels[i]
		}
	}
	result.Code, result.Message = "", ""
}

func (f *gpuFrame) draw(c driver.GPUCommand, res gpuResource) bool {
	e := f.encoder
	switch c.Op {
	case driver.GPUSetPipeline, driver.GPUSetVertexBuffer, driver.GPUSetIndexBuffer,
		driver.GPUSetUniformBuffer, driver.GPUSetTexture, driver.GPUSetSampler:
		if res.object == 0 {
			return false
		}
	}
	switch c.Op {
	case driver.GPUSetPipeline:
		f.pipeline = res.pipeline
		e.Send(sel("setRenderPipelineState:"), res.object)
		e.Send(sel("setDepthStencilState:"), res.depthState)
		e.Send(sel("setFrontFacingWinding:"), uint(1-f.pipeline.Winding))
		e.Send(sel("setCullMode:"), uint(f.pipeline.Cull))
	case driver.GPUSetVertexBuffer:
		e.Send(sel("setVertexBuffer:offset:atIndex:"), res.object, uint(c.Offset), uint(c.Slot))
	case driver.GPUSetIndexBuffer:
		f.indexBuffer, f.indexOffset, f.indexType = res.object, uint(c.Offset), uint(c.Slot)
	case driver.GPUSetUniformBuffer:
		const slot = uint(16)
		e.Send(sel("setVertexBuffer:offset:atIndex:"), res.object, uint(c.Offset), slot)
		e.Send(sel("setFragmentBuffer:offset:atIndex:"), res.object, uint(c.Offset), slot)
	case driver.GPUSetTexture:
		e.Send(sel("setVertexTexture:atIndex:"), res.object, uint(c.Slot))
		e.Send(sel("setFragmentTexture:atIndex:"), res.object, uint(c.Slot))
	case driver.GPUSetSampler:
		e.Send(sel("setVertexSamplerState:atIndex:"), res.object, uint(c.Slot))
		e.Send(sel("setFragmentSamplerState:atIndex:"), res.object, uint(c.Slot))
	case driver.GPUSetViewport:
		v := c.Viewport
		if float64(v[0])+float64(v[2]) > float64(f.width) || float64(v[1])+float64(v[3]) > float64(f.height) {
			return false
		}
		e.Send(sel("setViewport:"), gpuViewport{X: float64(v[0]), Y: float64(v[1]), Width: float64(v[2]),
			Height: float64(v[3]), MinDepth: float64(v[4]), MaxDepth: float64(v[5])})
	case driver.GPUSetScissor:
		s := c.Region
		if uint64(s.X)+uint64(s.Width) > uint64(f.width) || uint64(s.Y)+uint64(s.Height) > uint64(f.height) {
			return false
		}
		e.Send(sel("setScissorRect:"), gpuScissor{X: uint(s.X), Y: uint(s.Y), Width: uint(s.Width), Height: uint(s.Height)})
	case driver.GPUDrawVertices, driver.GPUDrawIndexed:
		topologies := [...]uint{3, 4, 0, 1, 2}
		primitive := topologies[f.pipeline.Topology]
		if c.Op == driver.GPUDrawVertices {
			e.Send(sel("drawPrimitives:vertexStart:vertexCount:instanceCount:baseInstance:"),
				primitive, uint(c.First), uint(c.Count), uint(c.Instances), uint(c.FirstInstance))
		} else {
			offset := f.indexOffset + uint(c.First)*(2<<f.indexType)
			e.Send(sel("drawIndexedPrimitives:indexCount:indexType:indexBuffer:indexBufferOffset:instanceCount:baseVertex:baseInstance:"),
				primitive, uint(c.Count), f.indexType, f.indexBuffer, offset, uint(c.Instances), int(c.BaseVertex), uint(c.FirstInstance))
		}
	}
	return true
}

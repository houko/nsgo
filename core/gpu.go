package core

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// GPUView is the core bridge for package gpu's View.
type GPUView struct {
	View         View
	Color, Depth GPUFormat
	OnEvent      func(*GPUDevice, GPUEventKind, GPUEvent)
}

type (
	// GPUFormat, GPUID and GPUCommand are the core protocol for package gpu.
	GPUFormat          = driver.GPUFormat
	GPUID              = driver.GPUID
	GPUCommand         = driver.GPUCommand
	GPUProgram         = shaderir.Program
	GPUSampler         = driver.GPUSampler
	GPUTexture         = driver.GPUTexture
	GPUVertexLayout    = driver.GPUVertexLayout
	GPUVertexAttribute = driver.GPUVertexAttribute
	GPUPipeline        = driver.GPUPipeline
	GPUPass            = driver.GPUPass
	GPURegion          = driver.GPURegion
	GPUVertexFormat    = driver.GPUVertexFormat
	GPUCompareFunc     = driver.GPUCompareFunc
	GPUEvent           = driver.GPUEvent
	GPUEventKind       = driver.EventKind
)

const (
	GPUReady                   = driver.EventGPUDeviceReady
	GPUResize                  = driver.EventGPUResize
	GPULost                    = driver.EventGPUDeviceLost
	GPUError                   = driver.EventGPUError
	GPUReadback                = driver.EventGPUReadback
	GPUTextureSampled          = driver.GPUTextureSampled
	GPUTextureRenderAttachment = driver.GPUTextureRenderAttachment
	GPUTextureCopySrc          = driver.GPUTextureCopySrc
	GPUTextureCopyDst          = driver.GPUTextureCopyDst
	GPUCreateBuffer            = driver.GPUCreateBuffer
	GPUCreateTexture           = driver.GPUCreateTexture
	GPUCreateSampler           = driver.GPUCreateSampler
	GPUCreatePipeline          = driver.GPUCreatePipeline
	GPURelease                 = driver.GPURelease
	GPUWriteBuffer             = driver.GPUWriteBuffer
	GPUWriteTexture            = driver.GPUWriteTexture
	GPUBeginPass               = driver.GPUBeginPass
	GPUEndPass                 = driver.GPUEndPass
	GPUSetPipeline             = driver.GPUSetPipeline
	GPUSetVertexBuffer         = driver.GPUSetVertexBuffer
	GPUSetIndexBuffer          = driver.GPUSetIndexBuffer
	GPUSetUniformBuffer        = driver.GPUSetUniformBuffer
	GPUSetTexture              = driver.GPUSetTexture
	GPUSetSampler              = driver.GPUSetSampler
	GPUSetViewport             = driver.GPUSetViewport
	GPUSetScissor              = driver.GPUSetScissor
	GPUDrawVertices            = driver.GPUDrawVertices
	GPUDrawIndexed             = driver.GPUDrawIndexed
	GPUReadTexture             = driver.GPUReadTexture
)

// GPUDevice is package gpu's connection to one device generation.
type GPUDevice struct {
	tree       *Tree
	node       NodeID
	generation uint64
	active     bool
}

// Submit sends ordered GPU commands for package gpu without a tree commit.
func (d *GPUDevice) Submit(commands []GPUCommand) {
	if d.active && len(commands) != 0 && d.tree.rt.submitGPU != nil {
		d.tree.rt.submitGPU(d.tree.id, d.node, d.generation, commands)
	}
}

// Active reports whether this GPU device generation can take commands.
func (d *GPUDevice) Active() bool { return d.active }

type gpuNode struct {
	config  driver.GPUView
	deliver func(*GPUDevice, GPUEventKind, GPUEvent)
	device  *GPUDevice
}

// Render implements Node for the GPU view bridge.
func (v GPUView) Render(*Scope) Node { return v }

func (v GPUView) mountIn(t *Tree, cx *Scope, parent *node) {
	if !cx.Env().SupportsGPU() || t.rt.submitGPU == nil {
		t.mount(cx, parent, v.View)
		if v.OnEvent != nil {
			unsupported := func() {
				v.OnEvent(nil, GPUError, GPUEvent{Code: "unsupported", Message: "the target has no GPU device"})
			}
			if t.rt.post != nil {
				cx.Post(unsupported)
			} else {
				unsupported()
			}
		}
		return
	}
	n := t.newNode(driver.KindGPU, parent, cx)
	n.props = driver.ViewProps{Label: v.View.Label, TestID: v.View.TestID}
	if v.View.Label != "" {
		n.props.Role = RoleImage
	}
	n.h = handlers{press: v.View.OnPress, layout: v.View.OnLayout}
	t.setClasses(n, v.View.Class)
	t.bindStyle(cx, n, v.View.Style, v.View.StyleFunc)
	t.setRef(n, v.View.Ref)
	for _, child := range v.View.Children {
		t.mount(cx, n, child)
	}
	t.rt.gpuGeneration++
	n.gpu = &gpuNode{
		config:  driver.GPUView{Generation: t.rt.gpuGeneration, ColorFormat: v.Color, DepthFormat: v.Depth},
		deliver: v.OnEvent,
	}
}

func (t *Tree) gpuEvent(ev driver.Event) {
	n := t.node(ev.ID)
	data, ok := ev.Data.(GPUEvent)
	if !ok || n == nil || n.deleted || n.gpu == nil || n.gpu.config.Generation != data.Generation {
		return
	}
	g := n.gpu
	if ev.Kind == GPUReady && g.device == nil {
		g.device = &GPUDevice{tree: t, node: n.id, generation: data.Generation, active: true}
	}
	if ev.Kind == GPULost && g.device != nil {
		g.device.active = false
	}
	if g.deliver != nil {
		t.within(n.owner, func() { g.deliver(g.device, ev.Kind, data) })
	}
}

package gpu

import (
	"slices"

	"github.com/nsgo-dev/nsgo/core"
)

// ShaderModule holds a portable Kage vertex and fragment program.
type ShaderModule struct{ program *core.GPUProgram }

// CompileKage compiles Vertex and Fragment. Texture0At through Texture3At
// sample a bound texture using normalized, top-left UV coordinates.
func CompileKage(source string) (*ShaderModule, error) {
	program, err := core.CompileGPUKage(source)
	if err != nil {
		return nil, Error{Code: ErrShaderCompile, Message: err.Error()}
	}
	return &ShaderModule{program: program}, nil
}

type ShaderStage struct {
	Module     *ShaderModule
	EntryPoint string
}

type VertexFormat = core.GPUVertexFormat

const (
	VertexFloat32 VertexFormat = iota + 1
	VertexFloat32x2
	VertexFloat32x3
	VertexFloat32x4
	VertexUint32
)

type VertexAttribute = core.GPUVertexAttribute

type VertexStepMode uint8

const (
	StepVertex VertexStepMode = iota
	StepInstance
)

type VertexBufferLayout struct {
	Stride     uint32
	StepMode   VertexStepMode
	Attributes []VertexAttribute
}
type PrimitiveTopology uint8

const (
	TriangleList PrimitiveTopology = iota
	TriangleStrip
	PointList
	LineList
	LineStrip
)

type FrontFace uint8

const (
	FrontCCW FrontFace = iota
	FrontCW
)

type CullMode uint8

const (
	CullNone CullMode = iota
	CullFront
	CullBack
)

type PrimitiveState struct {
	Topology  PrimitiveTopology
	FrontFace FrontFace
	CullMode  CullMode
}

type CompareFunc = core.GPUCompareFunc

const (
	CompareAlways CompareFunc = iota
	CompareNever
	CompareLess
	CompareLessEqual
	CompareEqual
	CompareGreaterEqual
	CompareGreater
	CompareNotEqual
)

type DepthStencilState struct {
	Format       TextureFormat
	DepthWrite   bool
	DepthCompare CompareFunc
}
type ColorTargetState struct {
	Format TextureFormat
}
type PipelineDesc struct {
	Label            string
	Vertex, Fragment ShaderStage
	Layouts          []*BindGroupLayout
	VertexBuffers    []VertexBufferLayout
	Primitive        PrimitiveState
	DepthStencil     *DepthStencilState
	Targets          []ColorTargetState
	SampleCount      int
}
type Pipeline struct {
	resource
	color, depth TextureFormat
	layouts      []*BindGroupLayout
	vertex       []core.GPUVertexLayout
}

type BindingKind uint8

const (
	BindingUniformBuffer BindingKind = iota + 1
	BindingTexture
	BindingSampler
)

type BindGroupLayoutEntry struct {
	Binding uint32
	Kind    BindingKind
}

type BindGroupEntry struct {
	Binding uint32
	Buffer  *Buffer
	Offset  uint64
	Texture *Texture
	Sampler *Sampler
}

type BindGroupLayout struct {
	device  *Device
	closed  bool
	entries []BindGroupLayoutEntry
}

func (l *BindGroupLayout) Close() error {
	l.closed = true
	return nil
}

type BindGroup struct {
	layout  *BindGroupLayout
	closed  bool
	entries []BindGroupEntry
}

func (g *BindGroup) Close() error {
	g.closed = true
	return nil
}

func (d *Device) CreateBindGroupLayout(cx *core.Scope, entries []BindGroupLayoutEntry) (*BindGroupLayout, error) {
	entries = slices.Clone(entries)
	slices.SortFunc(entries, func(a, b BindGroupLayoutEntry) int {
		if a.Binding < b.Binding {
			return -1
		}
		if a.Binding > b.Binding {
			return 1
		}
		return 0
	})
	for i, entry := range entries {
		if entry.Kind < BindingUniformBuffer || entry.Kind > BindingSampler || (i > 0 && entries[i-1].Binding == entry.Binding) {
			return nil, invalid("invalid or duplicate bind group entry")
		}
	}
	l := &BindGroupLayout{device: d, entries: entries}
	cx.Cleanup(func() { l.Close() })
	return l, nil
}

func (d *Device) CreateBindGroup(cx *core.Scope, l *BindGroupLayout, entries []BindGroupEntry) (*BindGroup, error) {
	if l == nil || l.device != d || l.closed || len(entries) != len(l.entries) {
		return nil, invalid("bind group needs a live layout and all its bindings")
	}
	bound := make(map[uint32]BindGroupEntry, len(entries))
	for _, entry := range entries {
		bound[entry.Binding] = entry
	}
	g := &BindGroup{layout: l, entries: make([]BindGroupEntry, 0, len(entries))}
	for _, entry := range l.entries {
		value, ok := bound[entry.Binding]
		if !ok || !d.acceptBinding(entry.Kind, value) {
			return nil, invalid("invalid resource for binding %d", entry.Binding)
		}
		g.entries = append(g.entries, value)
	}
	cx.Cleanup(func() { g.Close() })
	return g, nil
}

func (d *Device) acceptBinding(kind BindingKind, e BindGroupEntry) bool {
	switch kind {
	case BindingUniformBuffer:
		return e.Buffer != nil && e.Buffer.live(d) && e.Buffer.usage&UsageUniform != 0 &&
			e.Offset%256 == 0 && e.Offset < e.Buffer.size && e.Texture == nil && e.Sampler == nil
	case BindingTexture:
		return e.Texture != nil && e.Texture.live(d) && e.Texture.desc.Usage&TextureSampled != 0 &&
			e.Buffer == nil && e.Sampler == nil
	case BindingSampler:
		return e.Sampler != nil && e.Sampler.live(d) && e.Buffer == nil && e.Texture == nil
	}
	return false
}

func vertexSize(f VertexFormat) uint32 {
	switch f {
	case VertexFloat32, VertexUint32:
		return 4
	case VertexFloat32x2:
		return 8
	case VertexFloat32x3:
		return 12
	case VertexFloat32x4:
		return 16
	}
	return 0
}

func vertexType(f VertexFormat) string {
	switch f {
	case VertexFloat32:
		return "float"
	case VertexFloat32x2:
		return "vec2"
	case VertexFloat32x3:
		return "vec3"
	case VertexFloat32x4:
		return "vec4"
	case VertexUint32:
		return "uint"
	}
	return ""
}

func vertexLayouts(program *core.GPUProgram, layouts []VertexBufferLayout) ([]core.GPUVertexLayout, error) {
	out := make([]core.GPUVertexLayout, len(layouts))
	used := make([]bool, len(program.Attributes))
	for i, layout := range layouts {
		if layout.Stride == 0 || layout.Stride%4 != 0 || layout.StepMode > StepInstance || len(layout.Attributes) == 0 {
			return nil, invalid("invalid vertex buffer layout %d", i)
		}
		for _, a := range layout.Attributes {
			size := vertexSize(a.Format)
			if a.Location >= uint32(len(used)) || used[a.Location] || size == 0 || a.Offset%4 != 0 ||
				a.Offset > layout.Stride || size > layout.Stride-a.Offset ||
				program.Attributes[a.Location].String() != vertexType(a.Format) {
				return nil, invalid("vertex attribute %d does not match the shader input or stride", a.Location)
			}
			used[a.Location] = true
		}
		out[i] = core.GPUVertexLayout{Stride: layout.Stride, Instance: layout.StepMode == StepInstance,
			Attributes: slices.Clone(layout.Attributes)}
	}
	if slices.Contains(used, false) {
		return nil, invalid("a vertex shader input has no buffer attribute")
	}
	return out, nil
}

func (d *Device) CreatePipeline(cx *core.Scope, desc PipelineDesc) (*Pipeline, error) {
	if !d.connection.Active() {
		return nil, Error{Code: ErrDeviceLost, Message: "the GPU view no longer owns a device"}
	}
	if desc.Vertex.Module == nil || desc.Vertex.Module.program == nil || desc.Vertex.Module != desc.Fragment.Module ||
		(desc.Vertex.EntryPoint != "" && desc.Vertex.EntryPoint != "Vertex") ||
		(desc.Fragment.EntryPoint != "" && desc.Fragment.EntryPoint != "Fragment") ||
		desc.SampleCount < 0 || desc.SampleCount > 1 || len(desc.Targets) > 1 ||
		uint32(len(desc.VertexBuffers)) > d.Limits().MaxVertexBuffers ||
		desc.Primitive.Topology > LineStrip || desc.Primitive.FrontFace > FrontCW || desc.Primitive.CullMode > CullBack {
		return nil, invalid("invalid render pipeline descriptor")
	}
	program := desc.Vertex.Module.program
	if len(program.Attributes) > 31 {
		return nil, invalid("GPU pipelines support at most 31 vertex attributes")
	}
	color := d.format
	if len(desc.Targets) == 1 && desc.Targets[0].Format != FormatUndefined {
		color = desc.Targets[0].Format
	}
	if pixelBytes(color) == 0 {
		return nil, invalid("invalid pipeline color attachment")
	}
	var depth TextureFormat
	var compare CompareFunc
	var write bool
	if state := desc.DepthStencil; state != nil {
		if !depthFormat(state.Format) || state.DepthCompare > CompareNotEqual {
			return nil, invalid("invalid pipeline depth attachment")
		}
		depth, compare, write = state.Format, state.DepthCompare, state.DepthWrite
	}
	var bindings [4]int
	for _, layout := range desc.Layouts {
		if layout == nil || layout.device != d || layout.closed {
			return nil, invalid("pipeline layouts belong to the pipeline's device")
		}
		for _, entry := range layout.entries {
			bindings[entry.Kind]++
		}
	}
	uniforms := 0
	if len(program.Uniforms) != 0 {
		uniforms = 1
	}
	if bindings[BindingUniformBuffer] != uniforms || bindings[BindingTexture] != program.TextureCount ||
		bindings[BindingSampler] != program.TextureCount {
		return nil, invalid("Kage pipeline needs one binding per texture and sampler and one binding for all uniforms")
	}
	buffers, err := vertexLayouts(program, desc.VertexBuffers)
	if err != nil {
		return nil, err
	}
	p := &Pipeline{resource: d.allocate(), color: color, depth: depth,
		layouts: slices.Clone(desc.Layouts), vertex: buffers}
	cx.Cleanup(func() { p.Close() })
	d.send(core.GPUCommand{Op: core.GPUCreatePipeline, ID: p.id,
		Pipeline: &core.GPUPipeline{Program: program, ColorFormat: color, DepthFormat: depth,
			DepthCompare: compare, DepthWrite: write, VertexBuffers: buffers, Topology: uint8(desc.Primitive.Topology),
			Winding: uint8(desc.Primitive.FrontFace), Cull: uint8(desc.Primitive.CullMode)}})
	return p, nil
}

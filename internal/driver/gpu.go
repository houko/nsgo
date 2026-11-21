package driver

import "github.com/nsgo-dev/nsgo/internal/shaderir"

// GPUID names a resource on a GPU device. Zero stands for a surface attachment.
type GPUID uint32

type GPUFormat uint8

const (
	GPUFormatUndefined GPUFormat = iota
	GPUFormatRGBA8Unorm
	GPUFormatRGBA8UnormSRGB
	GPUFormatBGRA8Unorm
	GPUFormatRG8Unorm
	GPUFormatR8Unorm
	GPUFormatRGBA16Float
	GPUFormatDepth24Plus
	GPUFormatDepth24PlusStencil8
	GPUFormatDepth32Float
)

type GPUVertexFormat uint8

const (
	GPUVertexFloat32 GPUVertexFormat = iota + 1
	GPUVertexFloat32x2
	GPUVertexFloat32x3
	GPUVertexFloat32x4
	GPUVertexUint32
)

type GPUCompareFunc uint8

const (
	GPUCompareAlways GPUCompareFunc = iota
	GPUCompareNever
	GPUCompareLess
	GPUCompareLessEqual
	GPUCompareEqual
	GPUCompareGreaterEqual
	GPUCompareGreater
	GPUCompareNotEqual
)

type GPUOp uint8

const (
	GPUCreateBuffer GPUOp = iota + 1
	GPUCreateTexture
	GPUCreateSampler
	GPUCreatePipeline
	GPURelease
	GPUWriteBuffer
	GPUWriteTexture
	GPUBeginPass
	GPUEndPass
	GPUSetPipeline
	GPUSetVertexBuffer
	GPUSetIndexBuffer
	GPUSetUniformBuffer
	GPUSetTexture
	GPUSetSampler
	GPUSetViewport
	GPUSetScissor
	GPUDrawVertices
	GPUDrawIndexed
	GPUReadTexture
)

// GPUView configures the attachments of a mounted GPU surface. Generation
// distinguishes a new device from a device of a reused node ID.
type GPUView struct {
	Generation               uint64
	ColorFormat, DepthFormat GPUFormat
}

// GPUEvent carries a device event back to the graph.
type GPUEvent struct {
	Generation    uint64
	Width, Height int
	Format        GPUFormat
	Readback      uint32
	Pixels        []byte
	Code, Message string
}

const (
	GPUTextureSampled uint32 = 1 << iota
	GPUTextureRenderAttachment
	GPUTextureCopySrc
	GPUTextureCopyDst
)

type GPUTexture struct {
	Width, Height uint32
	Format        GPUFormat
	Usage         uint32
}

type GPUSampler struct {
	MinFilter, MagFilter uint8
	AddressU, AddressV   uint8
}

type GPUVertexAttribute struct {
	Location, Offset uint32
	Format           GPUVertexFormat
}

type GPUVertexLayout struct {
	Stride     uint32
	Instance   bool
	Attributes []GPUVertexAttribute
}

type GPUPipeline struct {
	Program                  *shaderir.Program
	VertexBuffers            []GPUVertexLayout
	ColorFormat, DepthFormat GPUFormat
	DepthCompare             GPUCompareFunc
	DepthWrite               bool
	Topology, Winding, Cull  uint8
}

type GPUPass struct {
	Color, Depth                                 GPUID
	ClearColor                                   Color
	ClearDepth                                   float64
	ColorLoad, ColorStore, DepthLoad, DepthStore uint8
	HasDepth                                     bool
}

type GPURegion struct {
	X, Y, Width, Height uint32
	BytesPerRow         uint32
}

// GPUCommand is an immutable operation submitted independently of view-tree commits.
type GPUCommand struct {
	Op                                           GPUOp
	ID                                           GPUID
	Slot, Count, First, Instances, FirstInstance uint32
	BaseVertex                                   int32
	Offset, Size                                 uint64
	Bytes                                        []byte
	Texture                                      *GPUTexture
	Sampler                                      *GPUSampler
	Pipeline                                     *GPUPipeline
	Pass                                         *GPUPass
	Region                                       GPURegion
	Viewport                                     [6]float32
}

// GPUSubmitter accepts one surface's ordered GPU operations on the engine goroutine.
type GPUSubmitter interface {
	SubmitGPU(id NodeID, generation uint64, commands []GPUCommand)
}

package gpu

import (
	"image"
	"math"
	"slices"

	"github.com/nsgo-dev/nsgo/core"
)

type BufferUsage uint32

const (
	UsageVertex BufferUsage = 1 << iota
	UsageIndex
	UsageUniform
	UsageStorage
	UsageCopySrc
	UsageCopyDst
)

type BufferDesc struct {
	Label string
	Size  uint64
	Usage BufferUsage
	Data  []byte
}

type TextureFormat = core.GPUFormat

const (
	FormatUndefined TextureFormat = iota
	FormatRGBA8Unorm
	FormatRGBA8UnormSRGB
	FormatBGRA8Unorm
	FormatRG8Unorm
	FormatR8Unorm
	FormatRGBA16Float
	FormatDepth24Plus
	FormatDepth24PlusStencil8
	FormatDepth32Float
)

func surfaceColor(f TextureFormat) bool {
	return f == FormatUndefined || f == FormatBGRA8Unorm
}

func depthFormat(f TextureFormat) bool {
	return f == FormatDepth24Plus || f == FormatDepth32Float
}

func pixelBytes(f TextureFormat) uint32 {
	switch f {
	case FormatR8Unorm:
		return 1
	case FormatRG8Unorm:
		return 2
	case FormatRGBA8Unorm, FormatRGBA8UnormSRGB, FormatBGRA8Unorm:
		return 4
	case FormatRGBA16Float:
		return 8
	}
	return 0
}

type TextureDimension uint8

const (
	Dim2D TextureDimension = iota
	DimCube
	Dim2DArray
)

type TextureUsage uint32

const (
	TextureSampled          TextureUsage = TextureUsage(core.GPUTextureSampled)
	TextureRenderAttachment TextureUsage = TextureUsage(core.GPUTextureRenderAttachment)
	TextureCopySrc          TextureUsage = TextureUsage(core.GPUTextureCopySrc)
	TextureCopyDst          TextureUsage = TextureUsage(core.GPUTextureCopyDst)
)

type TextureDesc struct {
	Label                            string
	Dimension                        TextureDimension
	Width, Height, Layers, MipLevels uint32
	SampleCount                      int
	Format                           TextureFormat
	Usage                            TextureUsage
}

type FilterMode uint8

const (
	FilterNearest FilterMode = iota
	FilterLinear
)

type AddressMode uint8

const (
	AddressClampToEdge AddressMode = iota
	AddressRepeat
	AddressMirrorRepeat
)

type SamplerDesc struct {
	MinFilter, MagFilter, MipmapFilter FilterMode
	AddressU, AddressV, AddressW       AddressMode
	Compare                            CompareFunc
	MaxAnisotropy                      uint16
}

type TextureRegion struct{ X, Y, Width, Height uint32 }
type TextureCopy = TextureRegion
type DataLayout struct{ BytesPerRow uint32 }

type resource struct {
	device *Device
	id     core.GPUID
	closed bool
}

func (r *resource) live(device *Device) bool {
	return r.device == device && !r.closed && device.connection.Active()
}

// Close releases the resource on the device's render queue.
func (r *resource) Close() error {
	if !r.closed {
		r.closed = true
		r.device.send(core.GPUCommand{Op: core.GPURelease, ID: r.id})
	}
	return nil
}

type Buffer struct {
	resource
	size  uint64
	usage BufferUsage
}
type Texture struct {
	resource
	desc TextureDesc
}
type Sampler struct{ resource }

type Limits struct {
	MaxTextureDimension2D uint32
	MaxBufferSize         uint64
	MaxVertexBuffers      uint32
}
type Features struct {
	StorageBuffers bool
	Compute        bool
}

// Device owns persistent GPU resources and runs methods on its graph's goroutine.
type Device struct {
	connection    *core.GPUDevice
	format, depth TextureFormat
	width, height int
	next          core.GPUID
	nextRead      uint32
	onError       func(Error)
	reads         map[uint32]func([]byte, error)
}

func (d *Device) Limits() Limits {
	return Limits{MaxTextureDimension2D: 8192, MaxBufferSize: 256 << 20, MaxVertexBuffers: 8}
}
func (d *Device) Features() Features               { return Features{} }
func (d *Device) SurfaceFormat() TextureFormat     { return d.format }
func (d *Device) SurfaceSize() (width, height int) { return d.width, d.height }

func (d *Device) send(command core.GPUCommand) {
	d.connection.Submit([]core.GPUCommand{command})
}

func (d *Device) fail(err Error) {
	if d.onError != nil {
		d.onError(err)
	}
}

func (d *Device) allocate() resource {
	d.next++
	return resource{device: d, id: d.next}
}

func (d *Device) CreateBuffer(cx *core.Scope, desc BufferDesc) (*Buffer, error) {
	if desc.Size == 0 || desc.Size > d.Limits().MaxBufferSize || uint64(len(desc.Data)) > desc.Size ||
		desc.Usage == 0 || desc.Usage & ^(UsageVertex|UsageIndex|UsageUniform|UsageCopySrc|UsageCopyDst) != 0 {
		return nil, invalid("invalid buffer size, usage or initial data")
	}
	r := &Buffer{resource: d.allocate(), size: desc.Size, usage: desc.Usage}
	cx.Cleanup(func() { r.Close() })
	d.send(core.GPUCommand{Op: core.GPUCreateBuffer, ID: r.id, Size: desc.Size, Bytes: slices.Clone(desc.Data)})
	return r, nil
}

func (d *Device) CreateTexture(cx *core.Scope, desc TextureDesc) (*Texture, error) {
	limit := d.Limits().MaxTextureDimension2D
	if desc.Dimension != Dim2D || desc.Width == 0 || desc.Height == 0 || desc.Width > limit ||
		desc.Height > limit || desc.Layers > 1 || desc.MipLevels > 1 || desc.SampleCount < 0 ||
		desc.SampleCount > 1 || desc.Usage == 0 || desc.Usage & ^(TextureSampled|TextureRenderAttachment|TextureCopySrc|TextureCopyDst) != 0 ||
		(pixelBytes(desc.Format) == 0 && (!depthFormat(desc.Format) || desc.Usage != TextureRenderAttachment)) {
		return nil, invalid("invalid 2D texture descriptor")
	}
	r := &Texture{resource: d.allocate(), desc: desc}
	cx.Cleanup(func() { r.Close() })
	d.send(core.GPUCommand{Op: core.GPUCreateTexture, ID: r.id,
		Texture: &core.GPUTexture{Width: desc.Width, Height: desc.Height, Format: desc.Format, Usage: uint32(desc.Usage)}})
	return r, nil
}

func (d *Device) CreateSampler(cx *core.Scope, desc SamplerDesc) (*Sampler, error) {
	if desc.MinFilter > FilterLinear || desc.MagFilter > FilterLinear || desc.AddressU > AddressMirrorRepeat ||
		desc.AddressV > AddressMirrorRepeat || desc.AddressW != AddressClampToEdge || desc.MipmapFilter != FilterNearest || desc.Compare != CompareAlways || desc.MaxAnisotropy > 1 {
		return nil, invalid("unsupported sampler descriptor")
	}
	r := &Sampler{resource: d.allocate()}
	cx.Cleanup(func() { r.Close() })
	d.send(core.GPUCommand{Op: core.GPUCreateSampler, ID: r.id,
		Sampler: &core.GPUSampler{MinFilter: uint8(desc.MinFilter), MagFilter: uint8(desc.MagFilter),
			AddressU: uint8(desc.AddressU), AddressV: uint8(desc.AddressV)}})
	return r, nil
}

func (d *Device) WriteBuffer(buf *Buffer, offset uint64, data []byte) {
	if buf == nil || !buf.live(d) || buf.usage&UsageCopyDst == 0 || offset > buf.size ||
		uint64(len(data)) > buf.size-offset || offset%4 != 0 || len(data)%4 != 0 {
		d.fail(invalid("buffer write requires matching device, copy destination usage and aligned bounds"))
		return
	}
	if len(data) != 0 {
		d.send(core.GPUCommand{Op: core.GPUWriteBuffer, ID: buf.id, Offset: offset, Bytes: slices.Clone(data)})
	}
}

func (t *Texture) contains(r TextureRegion) bool {
	return r.Width > 0 && r.Height > 0 && r.X <= t.desc.Width && r.Y <= t.desc.Height &&
		r.Width <= t.desc.Width-r.X && r.Height <= t.desc.Height-r.Y
}

func (d *Device) WriteTexture(tex *Texture, dst TextureCopy, data []byte, layout DataLayout) {
	if tex == nil || !tex.live(d) || tex.desc.Usage&TextureCopyDst == 0 || !tex.contains(dst) {
		d.fail(invalid("texture write requires matching device, copy destination usage and a valid region"))
		return
	}
	row := pixelBytes(tex.desc.Format) * dst.Width
	expected := uint64(layout.BytesPerRow)*uint64(dst.Height-1) + uint64(row)
	if row == 0 || layout.BytesPerRow < row || expected > uint64(len(data)) || expected > math.MaxInt {
		d.fail(invalid("invalid texture data layout"))
		return
	}
	d.send(core.GPUCommand{Op: core.GPUWriteTexture, ID: tex.id, Bytes: slices.Clone(data[:int(expected)]),
		Region: core.GPURegion{X: dst.X, Y: dst.Y, Width: dst.Width, Height: dst.Height, BytesPerRow: layout.BytesPerRow}})
}

func (d *Device) WriteTextureRGBA(tex *Texture, mipLevel, arrayLayer uint32, img *image.RGBA) {
	if img == nil || tex == nil || mipLevel != 0 || arrayLayer != 0 || pixelBytes(tex.desc.Format) != 4 ||
		tex.desc.Format == FormatBGRA8Unorm || img.Stride < 0 || uint64(img.Stride) > math.MaxUint32 {
		d.fail(invalid("RGBA upload needs a 2D RGBA8 texture and a valid image"))
		return
	}
	d.WriteTexture(tex, TextureCopy{Width: uint32(img.Rect.Dx()), Height: uint32(img.Rect.Dy())},
		img.Pix, DataLayout{BytesPerRow: uint32(img.Stride)})
}

func (d *Device) ReadTexture(tex *Texture, region TextureRegion, fn func(rgba []byte, err error)) {
	if fn == nil {
		return
	}
	if tex == nil || !tex.live(d) || tex.desc.Usage&TextureCopySrc == 0 || !tex.contains(region) ||
		pixelBytes(tex.desc.Format) != 4 || tex.desc.Format == FormatRG8Unorm || tex.desc.Format == FormatR8Unorm {
		fn(nil, invalid("texture readback needs a live RGBA8 or BGRA8 copy source and a valid region"))
		return
	}
	d.nextRead++
	d.reads[d.nextRead] = fn
	d.send(core.GPUCommand{Op: core.GPUReadTexture, ID: tex.id, Count: d.nextRead,
		Region: core.GPURegion{X: region.X, Y: region.Y, Width: region.Width, Height: region.Height}})
}

func (d *Device) readResult(event core.GPUEvent) {
	if fn := d.reads[event.Readback]; fn != nil {
		delete(d.reads, event.Readback)
		if event.Code != "" {
			fn(nil, Error{Code: ErrorCode(event.Code), Message: event.Message})
		} else {
			fn(event.Pixels, nil)
		}
	}
}

// Package gpu records render passes for a device-backed NSGo view.
package gpu

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/core"
)

type ErrorCode string

const (
	ErrUnsupported       ErrorCode = "unsupported"
	ErrInvalidDescriptor ErrorCode = "invalidDescriptor"
	ErrShaderCompile     ErrorCode = "shaderCompile"
	ErrOutOfMemory       ErrorCode = "outOfMemory"
	ErrDeviceLost        ErrorCode = "deviceLost"
	ErrFailed            ErrorCode = "failed"
)

// Error is a GPU validation or device failure.
type Error struct {
	Code    ErrorCode
	Message string
}

func (e Error) Error() string { return e.Message }

func invalid(format string, args ...any) Error {
	return Error{Code: ErrInvalidDescriptor, Message: fmt.Sprintf(format, args...)}
}

// DeviceLostEvent describes why this view's existing handles stopped working.
type DeviceLostEvent struct{ Reason string }

// View mounts a GPU surface below its ordinary children.
type View struct {
	ColorFormat, DepthFormat TextureFormat
	SampleCount              int
	OnReady                  func(*Device)
	OnResize                 func(width, height int)
	OnDeviceLost             func(DeviceLostEvent)
	OnError                  func(Error)
	Class                    string
	Style                    core.Style
	StyleFunc                func() core.Style
	Children                 []core.Node
	OnPress                  func(core.PressEvent)
	OnLayout                 func(core.Bounds)
	Ref                      *core.Ref
	Label, TestID            string
}

// Render connects the view to the graph.
func (v View) Render(cx *core.Scope) core.Node {
	base := core.View{Class: v.Class, Style: v.Style, StyleFunc: v.StyleFunc, Children: v.Children,
		OnPress: v.OnPress, OnLayout: v.OnLayout, Ref: v.Ref, Label: v.Label, TestID: v.TestID}
	if v.SampleCount < 0 || v.SampleCount > 1 || !surfaceColor(v.ColorFormat) ||
		(v.DepthFormat != FormatUndefined && !depthFormat(v.DepthFormat)) {
		if v.OnError != nil {
			cx.Post(func() { v.OnError(invalid("invalid GPU surface format or sample count")) })
		}
		return base
	}
	var device *Device
	return core.GPUView{View: base, Color: v.ColorFormat, Depth: v.DepthFormat,
		OnEvent: func(connection *core.GPUDevice, kind core.GPUEventKind, event core.GPUEvent) {
			switch kind {
			case core.GPUReady:
				device = &Device{connection: connection, format: event.Format, depth: v.DepthFormat,
					width: event.Width, height: event.Height, onError: v.OnError, reads: map[uint32]func([]byte, error){}}
				cx.Cleanup(func() { clear(device.reads) })
				if v.OnReady != nil {
					v.OnReady(device)
				}
			case core.GPUResize:
				if device != nil {
					device.width, device.height = event.Width, event.Height
				}
				if v.OnResize != nil {
					v.OnResize(event.Width, event.Height)
				}
			case core.GPULost:
				if device != nil {
					clear(device.reads)
				}
				if v.OnDeviceLost != nil {
					v.OnDeviceLost(DeviceLostEvent{Reason: event.Message})
				}
			case core.GPUReadback:
				if device != nil {
					device.readResult(event)
				}
			case core.GPUError:
				if v.OnError != nil {
					v.OnError(Error{Code: ErrorCode(event.Code), Message: event.Message})
				}
			}
		},
	}
}

//go:build darwin && !ios

package macos

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func (v *view) newGPU(config driver.GPUView) {
	dispatch := func(kind driver.EventKind, data driver.GPUEvent) {
		v.surface.engine.Dispatch(driver.Event{ID: v.id, Kind: kind, Data: data})
	}
	layer := apple.NewGPULayer(config, dispatch)
	if layer == nil {
		dispatch(driver.EventGPUError, driver.GPUEvent{Generation: config.Generation, Code: "unsupported", Message: "Metal is unavailable"})
		return
	}
	layer.Layer.Send(selActions, contentActions)
	layer.Layer.Send(sel("setZPosition:"), apple.Z(contentZ))
	v.native.Send(selLayer).Send(sel("addSublayer:"), layer.Layer)
	v.gpu = layer
	v.surface.gpuLayers.Store(v.id, layer)
}

func (v *view) sizeGPU() {
	w, h := float64(v.frame.Size.Width), float64(v.frame.Size.Height)
	apple.WithoutAnimation(func() {
		v.gpu.Layer.Send(selSetFrame, apple.Rect(0, 0, w, h))
		v.gpu.Resize(w, h, float64(v.surface.env.PixelRatio))
	})
}

func (s *surface) SubmitGPU(id driver.NodeID, generation uint64, commands []driver.GPUCommand) {
	if value, ok := s.gpuLayers.Load(id); ok {
		layer := value.(*apple.GPULayer)
		if layer.Generation == generation {
			layer.Submit(commands)
		}
	}
}

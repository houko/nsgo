//go:build !nsgo_noshader

package main

import (
	"runtime"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gpu"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestGPUFallbackKeepsChildren(t *testing.T) {
	fallback := make(chan gpu.Error, 1)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		if cx.Env().SupportsGPU() {
			return core.Text{Content: "Metal supported"}
		}
		return gpu.View{Children: []core.Node{core.Text{Content: "Fallback contents"}},
			OnError: func(err gpu.Error) { fallback <- err }}
	})
	if app.Exists("text:Metal supported") {
		t.Skip("the target has a GPU device")
	}
	app.WaitFor("text:Fallback contents", time.Second)
	app.WaitUntil(time.Second, func() bool { return len(fallback) != 0 })
	if err := <-fallback; err.Code != gpu.ErrUnsupported {
		t.Fatalf("error = %v, want unsupported", err)
	}
}

func TestGPUDisplaysMesh(t *testing.T) {
	if runtime.GOOS != "ios" {
		t.Skip("this test captures the Metal layer through UIKit")
	}
	ready := make(chan [2]int, 1)
	failed := make(chan gpu.Error, 10)
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return meshView{
			ready: func(d *gpu.Device) {
				width, height := d.SurfaceSize()
				ready <- [2]int{width, height}
			},
			err: func(err gpu.Error) {
				select {
				case failed <- err:
				default:
				}
			},
		}
	}, nsgotest.Size(320, 240))
	app.WaitUntil(5*time.Second, func() bool { return len(ready) != 0 || len(failed) != 0 })
	select {
	case err := <-failed:
		if err.Code == gpu.ErrUnsupported {
			t.Skip(err)
		}
		t.Fatal(err)
	case size := <-ready:
		if size[0] <= 0 || size[1] <= 0 {
			t.Fatalf("initial drawable size = %v", size)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(failed) != 0 {
			t.Fatal(<-failed)
		}
		img := app.Capture()
		blue, gold := 0, 0
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y += 4 {
			for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x += 4 {
				r, g, b, _ := img.At(x, y).RGBA()
				if r < g && g+5000 < b && g > 12000 {
					blue++
				}
				if b+5000 < g && g+5000 < r {
					gold++
				}
			}
		}
		if blue > 30 && gold > 30 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("the window did not show both sampled texture colors")
}

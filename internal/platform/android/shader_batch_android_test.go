//go:build android

package android

import (
	"bufio"
	"image"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestMountFlushesDeferredShaderPixels(t *testing.T) {
	pipe, err := os.Create(filepath.Join(t.TempDir(), "batches"))
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()

	const root, first = driver.NodeID(1), driver.NodeID(2)
	var batch driver.MountBatch
	batch.Root = root
	for i := range 3 {
		batch.Items = append(batch.Items, driver.MountItem{Kind: driver.ItemShader, ID: first + driver.NodeID(i), Shader: &driver.Shader{
			Role: driver.ShaderFill, Images: []driver.ImageSource{{Pixels: image.NewRGBA(image.Rect(0, 0, 2048, 1536)), Version: 1}},
		}})
	}
	s := surface{out: pipe, main: treeView{defaultsSent: true}}
	s.mount(&s.main, &batch)
	if _, err := pipe.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	wire := bufio.NewReader(pipe)
	const bytesPerImage = 12 << 20
	for i, want := range []struct{ items, pixels int }{{3, 2}, {1, 1}} {
		kind, payload, err := readMessage(wire)
		if err != nil {
			t.Fatalf("batch %d: %v", i, err)
		}
		r := reader{buf: payload}
		if kind != msgBatch || r.u32() != 0 || r.u32() != uint32(root) || r.byte() != 0 || r.byte() != 0 {
			t.Fatalf("batch %d has an unexpected header", i)
		}
		if n := r.u32(); n != uint32(want.items) {
			t.Fatalf("batch %d has %d items, want %d", i, n, want.items)
		}
		if n := len(payload); n < want.pixels*bytesPerImage || n >= (want.pixels+1)*bytesPerImage || n > maxMessage {
			t.Fatalf("batch %d has %d bytes, want %d images plus metadata", i, n, want.pixels)
		}
		if i > 0 && (r.byte() != byte(driver.ItemShader) || r.u32() != uint32(first+2)) {
			t.Fatal("the follow-up did not carry just the last shader")
		}
	}
	if _, err := wire.Peek(1); err != io.EOF {
		t.Fatalf("unexpected third batch, read error %v", err)
	}
	for _, it := range batch.Items {
		if held := s.main.shaderPixels[it.ID]; len(held) != 1 || held[0] != it.Shader.Images[0].Pixels {
			t.Errorf("shader %d was not recorded as sent", it.ID)
		}
	}
}

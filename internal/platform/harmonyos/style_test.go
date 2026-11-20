//go:build js && wasm

package harmonyos

import (
	"encoding/json"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestStyleSendsClip checks that a view whose only style is clipping, as a plain Overflow: Hidden container is, still writes its style operation, with the clip where Index.ets reads it: o[7] hidden, o[8] clip, then the transform.
func TestStyleSendsClip(t *testing.T) {
	m := &mounter{views: map[driver.NodeID]*view{}}
	m.style(3, &driver.ViewStyle{Clip: true})
	var got [][]any
	if err := json.Unmarshal(m.o.bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("a clip-only style wrote %d operations, want 1: %v", len(got), got)
	}
	o := got[0]
	if len(o) != 14 || o[0] != "s" {
		t.Fatalf("style operation %v, want s with 12 arguments", o)
	}
	if o[7] != false || o[8] != true {
		t.Fatalf("hidden %v and clip %v, want false and true", o[7], o[8])
	}
	if o[11] != float64(1) || o[12] != float64(1) {
		t.Fatalf("scale %v, %v after the clip, want 1, 1", o[11], o[12])
	}
}

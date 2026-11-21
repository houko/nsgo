package driver

import "testing"

func TestTextBackgroundCarriesGradients(t *testing.T) {
	gs := GradientList{{Kind: GradientLinear, Angle: 180, Stops: []ColorStop{{Color: RGBA(0, 0), At: Pct(62)}, {Color: RGB(0xffd60a), At: Pct(62)}}}}
	b := TextBackground{Color: RGB(0x123456), Radius: 4, Gradients: &gs}
	if len(*b.Gradients) != 1 || (*b.Gradients)[0].Angle != 180 || len((*b.Gradients)[0].Stops) != 2 {
		t.Fatalf("background gradient = %+v", b.Gradients)
	}
}

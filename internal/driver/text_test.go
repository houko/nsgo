package driver

import "testing"

func TestTextBackgroundVerticalBounds(t *testing.T) {
	for _, tc := range []struct {
		name        string
		top, bottom Length
		wantTop     Points
		wantBottom  Points
	}{
		{name: "full", wantBottom: 20},
		{name: "mixed", top: Pct(60), bottom: Pt(2), wantTop: 12, wantBottom: 18},
		{name: "negative", top: Pt(-2), bottom: Pct(-10), wantBottom: 20},
		{name: "overlap", top: Pct(60), bottom: Pct(40), wantTop: 12, wantBottom: 12},
		{name: "outside", top: Pt(25), bottom: Pct(110), wantTop: 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bg := TextBackground{Top: tc.top, Bottom: tc.bottom}
			top, bottom := bg.VerticalBounds(20)
			if top != tc.wantTop || bottom != tc.wantBottom {
				t.Errorf("bounds = (%v, %v), want (%v, %v)", top, bottom, tc.wantTop, tc.wantBottom)
			}
		})
	}
}

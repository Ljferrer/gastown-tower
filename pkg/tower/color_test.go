package tower

import "testing"

func TestGroupColor(t *testing.T) {
	if GroupColor("town") != TownColor {
		t.Errorf("town should get reserved amber")
	}
	// stable across calls
	if GroupColor("GigaClip") != GroupColor("GigaClip") {
		t.Errorf("rig color must be deterministic")
	}
	// rigs draw from the palette
	c := GroupColor("GigaClip")
	found := false
	for _, p := range rigPalette {
		if p == c {
			found = true
		}
	}
	if !found {
		t.Errorf("rig color %q not in palette", c)
	}
	// a rig never gets the reserved town color collision-free is not guaranteed,
	// but town must never be a palette hue
	for _, p := range rigPalette {
		if p == TownColor {
			t.Errorf("palette must not contain the reserved town color")
		}
	}
}

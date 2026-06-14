package tower

import (
	"reflect"
	"testing"
)

func TestClassifyPath(t *testing.T) {
	cases := []struct {
		segs []string
		want AgentRef
		ok   bool
	}{
		{[]string{"mayor"}, AgentRef{Name: "mayor", Role: "mayor", Group: "town", Addr: "mayor"}, true},
		{[]string{"deacon"}, AgentRef{Name: "deacon", Role: "deacon", Group: "town", Addr: "deacon"}, true},
		{[]string{"deacon", "dogs", "alpha"}, AgentRef{Name: "deacon/alpha", Role: "dog", Group: "town", Addr: "deacon/dogs/alpha"}, true},
		{[]string{"GigaClip", "witness"}, AgentRef{Name: "witness", Role: "witness", Rig: "GigaClip", Group: "GigaClip", Addr: "GigaClip/witness"}, true},
		{[]string{"GigaClip", "refinery", "rig"}, AgentRef{Name: "refinery", Role: "refinery", Rig: "GigaClip", Group: "GigaClip", Addr: "GigaClip/refinery"}, true},
		{[]string{"GigaClip", "polecats", "jasper", "GigaClip"}, AgentRef{Name: "jasper", Role: "polecat", Rig: "GigaClip", Group: "GigaClip", Addr: "GigaClip/polecats/jasper"}, true},
		{[]string{"GigaClip", "crew", "Quasimodo"}, AgentRef{Name: "Quasimodo", Role: "crew", Rig: "GigaClip", Group: "GigaClip", Addr: "GigaClip/crew/Quasimodo"}, true},
		// Nun audit seats: <rig>/seats/<Name>/<repo>, proper name preserved here.
		{[]string{"GigaClip", "seats", "Mary", "GigaClip"}, AgentRef{Name: "Mary", Role: "seat", Rig: "GigaClip", Group: "GigaClip", Addr: "GigaClip/seats/Mary"}, true},
		{[]string{"GigaClip", "seats", "Agnes", "GigaClip"}, AgentRef{Name: "Agnes", Role: "seat", Rig: "GigaClip", Group: "GigaClip", Addr: "GigaClip/seats/Agnes"}, true},
		{[]string{}, AgentRef{}, false},
	}
	for _, c := range cases {
		got, ok := classifyPath(c.segs)
		if ok != c.ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("classifyPath(%v) = %+v,%v; want %+v,%v", c.segs, got, ok, c.want, c.ok)
		}
	}
}

func TestSegmentsBelow(t *testing.T) {
	town := "-Users-example-gt"
	segs, ok := segmentsBelow("-Users-example-gt-GigaClip-witness", town)
	if !ok || !reflect.DeepEqual(segs, []string{"GigaClip", "witness"}) {
		t.Errorf("got %v,%v", segs, ok)
	}
	if _, ok := segmentsBelow("-Users-someoneelse-repo", town); ok {
		t.Errorf("foreign project should not match town")
	}
	if segs, ok := segmentsBelow(town, town); !ok || segs != nil {
		t.Errorf("town root: got %v,%v want nil,true", segs, ok)
	}
}

func TestSlugify(t *testing.T) {
	if got := slugify("/Users/example/gt"); got != "-Users-example-gt" {
		t.Fatalf("slugify = %q", got)
	}
}

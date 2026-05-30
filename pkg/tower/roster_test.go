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
		{[]string{"mayor"}, AgentRef{Name: "mayor", Role: "mayor", Group: "town"}, true},
		{[]string{"deacon"}, AgentRef{Name: "deacon", Role: "deacon", Group: "town"}, true},
		{[]string{"deacon", "dogs", "alpha"}, AgentRef{Name: "deacon/alpha", Role: "dog", Group: "town"}, true},
		{[]string{"GigaClip", "witness"}, AgentRef{Name: "witness", Role: "witness", Rig: "GigaClip", Group: "GigaClip"}, true},
		{[]string{"GigaClip", "refinery", "rig"}, AgentRef{Name: "refinery", Role: "refinery", Rig: "GigaClip", Group: "GigaClip"}, true},
		{[]string{"GigaClip", "polecats", "jasper", "GigaClip"}, AgentRef{Name: "jasper", Role: "polecat", Rig: "GigaClip", Group: "GigaClip"}, true},
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
	town := "-Users-lukasferrer-gt"
	segs, ok := segmentsBelow("-Users-lukasferrer-gt-GigaClip-witness", town)
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
	if got := slugify("/Users/lukasferrer/gt"); got != "-Users-lukasferrer-gt" {
		t.Fatalf("slugify = %q", got)
	}
}

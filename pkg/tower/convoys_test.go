package tower

import (
	"testing"
	"time"
)

func TestParseConvoys(t *testing.T) {
	in := []byte(`[
		{"id":"hq-cv-c2ld2","title":"Work: Phase 2","status":"open","completed":0,"total":1,
		 "created_at":"2026-06-14T03:41:38Z",
		 "tracked":[{"id":"gtt-975.5","title":"Phase 2","status":"hooked","assignee":"GasTownTower/polecats/slit","blocked":true}]},
		{"id":"hq-cv-2jktm","title":"Work: Layout","status":"closed","completed":1,"total":1,
		 "created_at":"2026-06-14T03:31:52Z",
		 "tracked":[{"id":"gtt-975.1","title":"Layout","status":"closed","assignee":"GasTownTower/polecats/furiosa"}]}
	]`)
	cvs, err := parseConvoys(in)
	if err != nil {
		t.Fatalf("parseConvoys: %v", err)
	}
	if len(cvs) != 2 {
		t.Fatalf("len = %d, want 2", len(cvs))
	}
	c0 := cvs[0]
	if c0.ID != "hq-cv-c2ld2" || c0.Status != "open" || c0.Total != 1 {
		t.Errorf("convoy[0] = %+v", c0)
	}
	if c0.CreatedAt.IsZero() {
		t.Error("convoy[0] created_at not parsed")
	}
	if len(c0.Tracked) != 1 || c0.Tracked[0].Assignee != "GasTownTower/polecats/slit" || !c0.Tracked[0].Blocked {
		t.Errorf("convoy[0] tracked = %+v", c0.Tracked)
	}
	if cvs[1].Status != "closed" || cvs[1].Completed != 1 {
		t.Errorf("convoy[1] = %+v", cvs[1])
	}
}

func TestParseConvoysInvalid(t *testing.T) {
	if _, err := parseConvoys([]byte("{not array}")); err == nil {
		t.Fatal("want error for invalid json, got nil")
	}
}

func TestParseMergeQueue(t *testing.T) {
	in := []byte(`[
		{"id":"gtt-wisp-p7i","title":"Merge: gtt-975.1","status":"open",
		 "description":"branch: polecat/furiosa/gtt-975.1@x\ntarget: main\nsource_issue: gtt-975.1\nrig: GasTownTower\nworker: furiosa\npre_verified: true",
		 "created_at":"2026-06-14T03:38:20Z"}
	]`)
	mrs, err := parseMergeQueue(in, "GasTownTower")
	if err != nil {
		t.Fatalf("parseMergeQueue: %v", err)
	}
	if len(mrs) != 1 {
		t.Fatalf("len = %d, want 1", len(mrs))
	}
	m := mrs[0]
	if m.SourceIssue != "gtt-975.1" || m.Worker != "furiosa" || m.Rig != "GasTownTower" {
		t.Errorf("mr = %+v", m)
	}
	if m.Status != "open" || m.CreatedAt.IsZero() {
		t.Errorf("mr status/time = %q/%v", m.Status, m.CreatedAt)
	}
}

// An empty merge queue serializes as JSON null; that must yield an empty slice,
// not an error.
func TestParseMergeQueueNull(t *testing.T) {
	mrs, err := parseMergeQueue([]byte("null"), "GasTownTower")
	if err != nil {
		t.Fatalf("parseMergeQueue(null): %v", err)
	}
	if len(mrs) != 0 {
		t.Fatalf("len = %d, want 0", len(mrs))
	}
}

func TestDescField(t *testing.T) {
	desc := "branch: b\nsource_issue: gtt-9\nworker: nux\n"
	if got := descField(desc, "source_issue"); got != "gtt-9" {
		t.Errorf("source_issue = %q, want gtt-9", got)
	}
	if got := descField(desc, "worker"); got != "nux" {
		t.Errorf("worker = %q, want nux", got)
	}
	if got := descField(desc, "absent"); got != "" {
		t.Errorf("absent = %q, want empty", got)
	}
}

func TestRecentConvoys(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	all := []Convoy{
		{ID: "old-closed", Status: "closed", CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "new-closed", Status: "closed", CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "open-old", Status: "open", CreatedAt: now.Add(-72 * time.Hour)},
		{ID: "open-new", Status: "open", CreatedAt: now.Add(-1 * time.Hour)},
	}
	got := recentConvoys(all, now, 24*time.Hour)
	// old-closed is dropped (beyond window); open ones always kept regardless of age.
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (%+v)", len(got), ids(got))
	}
	// Order: open-first then newest-first → open-new, open-old, new-closed.
	wantOrder := []string{"open-new", "open-old", "new-closed"}
	for i, w := range wantOrder {
		if got[i].ID != w {
			t.Errorf("order[%d] = %q, want %q (got %v)", i, got[i].ID, w, ids(got))
		}
	}
}

func ids(cvs []Convoy) []string {
	out := make([]string, len(cvs))
	for i, c := range cvs {
		out[i] = c.ID
	}
	return out
}

func TestRigNames(t *testing.T) {
	prefixes := map[string]string{
		".":                       "hq",
		"GasTownTower":            "gtt",
		"healthcare":              "he",
		"LokustGasTown/mayor/rig": "lgt",
	}
	got := rigNames(prefixes)
	want := []string{"GasTownTower", "LokustGasTown", "healthcare"} // sorted, town "." excluded, first segment only
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("rigNames[%d] = %q, want %q (full %v)", i, got[i], want[i], got)
		}
	}
}

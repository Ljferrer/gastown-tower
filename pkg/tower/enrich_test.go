package tower

import "testing"

func TestParseCharsheet(t *testing.T) {
	in := []byte(`{"Handle":"furiosa","Tier":"Bronze","StampCount":7,"ClusterBreadth":3}`)
	rep, err := parseCharsheet(in)
	if err != nil {
		t.Fatalf("parseCharsheet: %v", err)
	}
	if rep.Handle != "furiosa" || rep.Tier != "Bronze" {
		t.Errorf("handle/tier = %q/%q, want furiosa/Bronze", rep.Handle, rep.Tier)
	}
	if rep.Stamps != 7 || rep.Clusters != 3 {
		t.Errorf("stamps/clusters = %d/%d, want 7/3", rep.Stamps, rep.Clusters)
	}
}

func TestParseCharsheetInvalid(t *testing.T) {
	if _, err := parseCharsheet([]byte("not json")); err == nil {
		t.Fatal("want error for invalid json, got nil")
	}
}

func TestParseMailInbox(t *testing.T) {
	cases := []struct {
		name          string
		in            string
		total, unread int
	}{
		{"plural with unread", "📬 Inbox: mayor/ (5 messages, 2 unread)", 5, 2},
		{"singular", "📬 Inbox: GasTownTower/witness (1 message, 0 unread)", 1, 0},
		{"empty inbox", "📬 Inbox: x (0 messages, 0 unread)\n\n  (no messages)", 0, 0},
		{"no match", "totally unrelated text", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := parseMailInbox(c.in)
			if m.Total != c.total || m.Unread != c.unread {
				t.Errorf("got %d/%d, want %d/%d", m.Unread, m.Total, c.unread, c.total)
			}
		})
	}
}

func TestParseHooks(t *testing.T) {
	in := []byte(`[
		{"id":"gtt-vsq","title":"Slice 3","assignee":"GasTownTower/polecats/furiosa"},
		{"id":"gtt-vlx","title":"Slice 4","assignee":"GasTownTower/polecats/nux"},
		{"id":"gtt-zzz","title":"orphan","assignee":""}
	]`)
	hooks, err := parseHooks(in)
	if err != nil {
		t.Fatalf("parseHooks: %v", err)
	}
	if len(hooks) != 2 {
		t.Fatalf("len = %d, want 2 (empty assignee skipped)", len(hooks))
	}
	if got := hooks["GasTownTower/polecats/furiosa"]; got != "gtt-vsq: Slice 3" {
		t.Errorf("furiosa hook = %q", got)
	}
	if got := hooks["GasTownTower/polecats/nux"]; got != "gtt-vlx: Slice 4" {
		t.Errorf("nux hook = %q", got)
	}
}

func TestParseHooksInvalid(t *testing.T) {
	if _, err := parseHooks([]byte("{not an array}")); err == nil {
		t.Fatal("want error for invalid json, got nil")
	}
}

package app

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAppliedExpiryAndServerSummary(t *testing.T) {
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	raw := func(before, after time.Time) string {
		data, _ := json.Marshal(map[string]time.Time{"not_before": before, "not_after": after})
		return string(data)
	}
	expired := raw(now.AddDate(0, 0, -100), now.AddDate(0, 0, -1))
	warning := raw(now.AddDate(0, 0, -90), now.AddDate(0, 0, 10))
	healthy := raw(now.AddDate(0, 0, -30), now.AddDate(0, 0, 60))
	for _, tc := range []struct {
		active any
		raw    any
		state  string
		days   int
	}{
		{nil, nil, "Not deployed", 0}, {"v", nil, "Unknown", 0}, {"v", "bad", "Unknown", 0},
		{"v", expired, "Expired", -1}, {"v", warning, "Warning", 10}, {"v", healthy, "Healthy", 60},
	} {
		info := certificateExpiry(tc.active, tc.raw, 30, now)
		if info.State != tc.state || info.DaysRemaining != tc.days || info.RemainingPercent < 0 || info.RemainingPercent > 100 {
			t.Fatalf("expiry %+v", info)
		}
	}
	targets := []map[string]any{
		{"server_id": "s1", "active_version": "old-applied", "active_metadata": warning},
		{"server_id": "s1", "active_version": "other", "active_metadata": healthy},
		{"server_id": "s1", "active_version": nil},
		{"server_id": "s1", "active_version": "unavailable"},
	}
	summary := summarizeExpiry(targets, 30, now)["s1"]
	if summary.DaysRemaining != 10 || summary.Total != 4 || summary.Known != 2 || summary.NotDeployed != 1 || summary.Unknown != 1 {
		t.Fatalf("summary %+v", summary)
	}
}

func TestTreeExpiryUsesAppliedVersion(t *testing.T) {
	f := fixture(t)
	a := testApp(t, f)
	c := login(t, a)
	version, target, environment := register(t, c, f)
	if _, err := a.Store.DB.Exec("UPDATE deployment_targets SET active_version=? WHERE id=?", version, target); err != nil {
		t.Fatal(err)
	}
	// Upload a newer version without deploying it.
	if _, err := a.Store.Import(environment, f.Second, "", a.Roots); err != nil {
		t.Fatal(err)
	}
	tree, err := a.BuildTree()
	if err != nil {
		t.Fatal(err)
	}
	var found *Node
	var walk func(*Node)
	walk = func(n *Node) {
		if n.Type == "target" && n.EntityID == target {
			found = n
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(tree)
	if found == nil {
		t.Fatal("target missing")
	}
	bundle, _, err := a.Store.Bundle(version)
	if err != nil {
		t.Fatal(err)
	}
	info := found.Data["expiry"].(expiryInfo)
	if info.NotAfter != bundle.Leaf.NotAfter.UTC().Format(time.RFC3339) {
		t.Fatal("expiry used uploaded version instead of applied version")
	}
}

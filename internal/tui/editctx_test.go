package tui

import "testing"

func TestEditContextMenu(t *testing.T) {
	root := fixture(t)
	m := keys(open(t, root, 100, 30), "tab", "e")
	v := plain(m)
	mustContain(t, v, "Edit context", "Rename context prod-eu", "Rename cluster eks-eu")
	mustContain(t, lastLine(v), "r rename context", "c rename cluster", "esc cancel")
	m = keys(m, "esc")
	if len(m.dialogs) != 0 {
		t.Fatal("esc must close the menu")
	}
}

func TestRenameContextFromTUI(t *testing.T) {
	root := fixture(t)
	m := keys(open(t, root, 100, 30), "tab", "e", "r")
	mustContain(t, plain(m), "Rename context", "New name for context prod-eu", "> prod-eu")

	m = keys(m, "backspace", "backspace")
	m = typeText(m, "us")
	mustContain(t, plain(m), "✗ Context prod-us already exists")
	m = keys(m, "enter")
	if len(m.dialogs) != 1 {
		t.Fatal("enter must be disabled while the name is taken")
	}

	m = keys(m, "backspace", "backspace")
	m = typeText(m, "europe")
	m = keys(m, "enter")
	mustContain(t, plain(m), "✓ Renamed context prod-eu → prod-europe")
	cfg := loadProject(t, root, "prod")
	if cfg.CurrentContext != "prod-europe" || cfg.Contexts["prod-eu"] != nil {
		t.Fatalf("current = %q, contexts = %v", cfg.CurrentContext, cfg.Contexts)
	}
	if c, _ := m.selectedContext(); c.Name != "prod-europe" {
		t.Errorf("cursor on %q, want the renamed context", c.Name)
	}
}

func TestRenameContextShortcut(t *testing.T) {
	root := fixture(t)
	m := keys(open(t, root, 100, 30), "tab", "r")
	mustContain(t, plain(m), "Rename context", "New name for context prod-eu")
}

func TestRenameClusterFromTUI(t *testing.T) {
	root := fixture(t)
	m := keys(open(t, root, 100, 30), "tab", "e", "c")
	mustContain(t, plain(m), "Rename cluster", "New name for cluster eks-eu", "> eks-eu")
	for range len("eks-eu") {
		m = keys(m, "backspace")
	}
	m = typeText(m, "eks-us")
	mustContain(t, plain(m), "✗ Cluster eks-us already exists")
	m = keys(m, "backspace", "backspace")
	m = typeText(m, "frankfurt")
	m = keys(m, "enter")
	mustContain(t, plain(m), "✓ Renamed cluster eks-eu → eks-frankfurt (1 context updated)")
	cfg := loadProject(t, root, "prod")
	if cfg.Clusters["eks-frankfurt"] == nil || cfg.Clusters["eks-eu"] != nil {
		t.Fatal("cluster not renamed")
	}
	if got := cfg.Contexts["prod-eu"].Cluster; got != "eks-frankfurt" {
		t.Fatalf("prod-eu points at %q", got)
	}
}

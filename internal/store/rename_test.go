package store

import (
	"testing"

	"github.com/nunoarmada/kxconfig/v2/internal/kubecfg"
)

func TestRenameContextAndClusterBackUpMaster(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", twoCtxCfg)
	tgt := Target{Path: s.ProjectPath("dev"), Project: "dev"}
	if err := s.RenameContext(tgt, "bare", "main"); err != nil {
		t.Fatal(err)
	}
	moved, err := s.RenameCluster(tgt, "c", "eks-dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 2 {
		t.Fatalf("moved = %v", moved)
	}
	cfg, err := kubecfg.Load(s.ProjectPath("dev"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentContext != "main" || cfg.Contexts["main"] == nil || cfg.Contexts["bare"] != nil {
		t.Fatalf("context not renamed: current %q", cfg.CurrentContext)
	}
	if cfg.Clusters["eks-dev"] == nil || cfg.Contexts["api"].Cluster != "eks-dev" {
		t.Fatal("cluster not renamed")
	}
	bs, err := s.Backups.List("dev")
	if err != nil {
		t.Fatal(err)
	}
	ops := map[string]bool{}
	for _, b := range bs {
		ops[b.Op] = true
	}
	if !ops["renamectx"] || !ops["renamecluster"] {
		t.Fatalf("backups = %+v", bs)
	}
}

func TestRenameContextFailureWritesNothing(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", twoCtxCfg)
	tgt := Target{Path: s.ProjectPath("dev"), Project: "dev"}
	if err := s.RenameContext(tgt, "bare", "api"); err == nil {
		t.Fatal("renaming onto a taken name must fail")
	}
	if bs, _ := s.Backups.List("dev"); len(bs) != 0 {
		t.Fatalf("backup taken for a failed edit: %+v", bs)
	}
}

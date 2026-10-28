package store

import (
	"testing"

	"github.com/nunoarmada/kxconfig/v2/internal/kubecfg"
)

const twoCtxCfg = `apiVersion: v1
kind: Config
clusters:
- name: c
  cluster: {server: "https://c.example:6443"}
users:
- name: u
  user: {token: t}
contexts:
- name: bare
  context: {cluster: c, user: u}
- name: api
  context: {cluster: c, user: u, namespace: api}
current-context: bare
`

func namespaceOf(t *testing.T, s *Store, project, ctx string) string {
	t.Helper()
	cfg, err := kubecfg.Load(s.ProjectPath(project))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Contexts[ctx].Namespace
}

func TestUseFillsDefaultNamespace(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", twoCtxCfg)
	if err := s.Use("dev"); err != nil {
		t.Fatal(err)
	}
	if ns := namespaceOf(t, s, "dev", "bare"); ns != "default" {
		t.Fatalf("namespace of current context = %q, want default", ns)
	}
	if ns := namespaceOf(t, s, "dev", "api"); ns != "api" {
		t.Fatalf("other context namespace changed to %q", ns)
	}
}

func TestSetContextFillsDefaultNamespaceOnlyWhenEmpty(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", twoCtxCfg)
	tgt := Target{Path: s.ProjectPath("dev"), Project: "dev"}
	if err := s.SetContext(tgt, "api"); err != nil {
		t.Fatal(err)
	}
	if ns := namespaceOf(t, s, "dev", "api"); ns != "api" {
		t.Fatalf("explicit namespace overwritten: %q", ns)
	}
	if err := s.SetContext(tgt, "bare"); err != nil {
		t.Fatal(err)
	}
	if ns := namespaceOf(t, s, "dev", "bare"); ns != "default" {
		t.Fatalf("empty namespace not filled: %q", ns)
	}
}

func TestUseWithoutContextsStillWorks(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	if err := s.Use("dev"); err != nil {
		t.Fatal(err)
	}
}

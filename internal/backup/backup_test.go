package backup

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newManager(t *testing.T, step time.Duration) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "src.yaml")
	if err := os.WriteFile(src, []byte("apiVersion: v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cur := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	m := &Manager{
		Dir: filepath.Join(root, "kx", "backups"),
		Now: func() time.Time {
			t := cur
			cur = cur.Add(step)
			return t
		},
	}
	return m, src
}

func TestCreateListNewestFirst(t *testing.T) {
	m, src := newManager(t, time.Second)
	a, err := m.Create("proj", "merge", src)
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Create("proj", "delete", src)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != "20261027T120000.000Z-merge" {
		t.Errorf("ID = %q", a.ID)
	}
	if a.Project != "proj" || a.Op != "merge" || a.Path != filepath.Join(m.Dir, "proj", a.ID+".yaml") {
		t.Errorf("unexpected backup %+v", a)
	}
	got, err := m.List("proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != b.ID || got[1].ID != a.ID {
		t.Fatalf("List = %+v", got)
	}
	if got[0].Op != "delete" || !got[0].Time.Equal(b.Time) {
		t.Errorf("parsed fields wrong: %+v", got[0])
	}
	data, err := os.ReadFile(got[1].Path)
	if err != nil || string(data) != "apiVersion: v1\n" {
		t.Errorf("content = %q, err %v", data, err)
	}
}

func TestListMissingDirEmpty(t *testing.T) {
	m, _ := newManager(t, time.Second)
	got, err := m.List("nope")
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestListIgnoresStrayFiles(t *testing.T) {
	m, src := newManager(t, time.Second)
	if _, err := m.Create("proj", "merge", src); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(m.Dir, "proj")
	for _, n := range []string{".tmp-123", "notes.txt", "garbage.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := m.List("proj")
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestPruneKeeps20(t *testing.T) {
	m, src := newManager(t, time.Second)
	var first Backup
	for i := 0; i < 25; i++ {
		b, err := m.Create("proj", "merge", src)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = b
		}
	}
	got, err := m.List("proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != Keep {
		t.Fatalf("len = %d, want %d", len(got), Keep)
	}
	if _, err := os.Stat(first.Path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("oldest backup still present: %v", err)
	}
}

func TestCreateSameInstantNoClobber(t *testing.T) {
	m, src := newManager(t, 0)
	var ids []string
	for i := 0; i < 3; i++ {
		b, err := m.Create("proj", "merge", src)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, b.ID)
	}
	want := []string{"20261027T120000.000Z-merge", "20261027T120000.000Z-merge-2", "20261027T120000.000Z-merge-3"}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("ids[%d] = %q, want %q", i, ids[i], want[i])
		}
	}
	got, err := m.List("proj")
	if err != nil || len(got) != 3 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if got[0].ID != want[2] || got[0].Op != "merge" {
		t.Errorf("newest = %+v", got[0])
	}
}

func TestBackupPerms(t *testing.T) {
	m, src := newManager(t, time.Second)
	b, err := m.Create("proj", "merge", src)
	if err != nil {
		t.Fatal(err)
	}
	check := func(p string, want fs.FileMode) {
		t.Helper()
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != want {
			t.Errorf("%s mode = %o, want %o", p, fi.Mode().Perm(), want)
		}
	}
	check(b.Path, 0o600)
	check(filepath.Dir(b.Path), 0o700)
	check(m.Dir, 0o700)
}

func TestInitialProjectAccepted(t *testing.T) {
	m, src := newManager(t, time.Second)
	b, err := m.Create("_initial", "adopt", src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get("_initial", b.ID); err != nil {
		t.Fatal(err)
	}
}

func TestGet(t *testing.T) {
	m, src := newManager(t, time.Second)
	b, err := m.Create("proj", "merge", src)
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Get("proj", b.ID)
	if err != nil || got.ID != b.ID || got.Path != b.Path {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestGetUnknown(t *testing.T) {
	m, _ := newManager(t, time.Second)
	for _, id := range []string{"nope", "../x", ""} {
		if _, err := m.Get("proj", id); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Get(%q) err = %v, want ErrNotExist", id, err)
		}
	}
}

package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/nunoarmada/kxconfig/v2/internal/backup"
	"github.com/nunoarmada/kxconfig/v2/internal/fsutil"
	"github.com/nunoarmada/kxconfig/v2/internal/kubecfg"
)

var (
	// ErrActive means the operation needs force because the project is active.
	ErrActive = errors.New("project is active")
	// ErrExists means a project with that name already exists.
	ErrExists = errors.New("project already exists")
)

// Env is the kx shell state read from the process environment.
type Env struct {
	InShell    bool   // KX_SHELL=1
	Project    string // KX_PROJECT
	Kubeconfig string // KUBECONFIG
}

// EnvFromOS reads KX_SHELL, KX_PROJECT and KUBECONFIG.
func EnvFromOS() Env {
	return Env{
		InShell:    os.Getenv("KX_SHELL") == "1",
		Project:    os.Getenv("KX_PROJECT"),
		Kubeconfig: os.Getenv("KUBECONFIG"),
	}
}

// classifyBackup interprets the result of Manager.Create. A non-empty Backup
// with an error means the backup file exists and only pruning old backups
// failed: the operation may proceed and the error is returned as a warning.
// Without a Backup the error is fatal and the caller must abort unchanged.
func classifyBackup(b backup.Backup, err error) (backup.Backup, string, error) {
	switch {
	case err == nil:
		return b, "", nil
	case b.ID != "":
		return b, err.Error(), nil
	default:
		return backup.Backup{}, "", err
	}
}

// backup takes a backup of the file at src. See classifyBackup for the
// contract; warn is a non-fatal message.
func (s *Store) backup(project, op, src string) (b backup.Backup, warn string, err error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return backup.Backup{}, "", fmt.Errorf("back up %s: %w", src, err)
	}
	return s.backupData(project, op, data, src)
}

// backupData is the single entry point for taking a backup before a change,
// of exactly the bytes the caller read (and parsed) from src.
func (s *Store) backupData(project, op string, data []byte, src string) (b backup.Backup, warn string, err error) {
	create := s.Backups.CreateData
	if s.create != nil {
		create = s.create
	}
	b, warn, err = classifyBackup(create(project, op, data))
	if err != nil {
		return backup.Backup{}, "", fmt.Errorf("back up %s: %w", src, err)
	}
	return b, warn, nil
}

// readProjectFile reads path once. A zero-length file is refused: it is what
// a concurrent writer leaves behind, and merging into it would silently
// replace the project.
func readProjectFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s: %w", path, ErrEmptyProject)
	}
	return data, nil
}

// BackupProject takes a backup of project's master file under op (for example
// "edit", before an external editor changes it). warn is a non-fatal message
// (pruning old backups failed).
func (s *Store) BackupProject(project, op string) (backup.Backup, string, error) {
	if err := ValidateName(project); err != nil {
		return backup.Backup{}, "", err
	}
	return s.backup(project, op, s.ProjectPath(project))
}

// ImportMode selects how Import combines the incoming file with a project.
type ImportMode int

// Import modes.
const (
	ImportMerge   ImportMode = iota // merge entities per MergeOptions
	ImportReplace                   // the flattened incoming file becomes the project
)

// ImportResult describes what Import did.
type ImportResult struct {
	Project  string
	Created  bool
	Changed  bool
	Report   kubecfg.MergeReport
	Backup   *backup.Backup // nil when no backup was taken
	Warnings []string       // non-fatal problems (e.g. pruning old backups failed)
}

// existingProject reports whether project's master exists as a regular file.
func (s *Store) existingProject(project string) (bool, error) {
	fi, err := os.Lstat(s.ProjectPath(project))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !fi.Mode().IsRegular() {
		return false, fmt.Errorf("%s is not a regular file", s.ProjectPath(project))
	}
	return true, nil
}

// Import merges (or replaces with) the kubeconfig in file into project. The
// project's master file (never <root>/config) is written, even when it is the
// active project. Nothing is written and no backup is taken when the result
// equals the current file or when conflicts resolve to PolicyFail (the error
// then wraps kubecfg.ErrConflicts and Report lists them).
func (s *Store) Import(file, project string, mode ImportMode, opts kubecfg.MergeOptions) (ImportResult, error) {
	res := ImportResult{Project: project}
	if err := ValidateName(project); err != nil {
		return res, err
	}
	incoming, err := kubecfg.LoadFlattened(file)
	if err != nil {
		return res, fmt.Errorf("read %s: %w", file, err)
	}
	path := s.ProjectPath(project)
	exists, err := s.existingProject(project)
	if err != nil {
		return res, err
	}

	if !exists {
		_, res.Report, err = kubecfg.Merge(nil, incoming, kubecfg.MergeOptions{})
		if err != nil {
			return res, err
		}
		if err := os.MkdirAll(s.Root, 0o700); err != nil {
			return res, err
		}
		if err := kubecfg.Save(incoming, path); err != nil {
			return res, err
		}
		res.Created, res.Changed = true, true
		return res, nil
	}

	// The merge result is built from the raw master so untouched entries
	// keep their file references; flattened copies are only used to compare.
	return res, fsutil.WithLock(path, func() error {
		data, err := readProjectFile(path)
		if err != nil {
			return err
		}
		master, err := kubecfg.LoadBytes(data, path)
		if err != nil {
			return fmt.Errorf("project %s is not a valid kubeconfig: %w", project, err)
		}
		result := incoming
		op := "replace"
		if mode == ImportMerge {
			op = "import"
			result, res.Report, err = kubecfg.Merge(master, incoming, opts)
			if err != nil {
				return err
			}
		}
		same, err := sameConfig(master, result)
		if err != nil {
			return err
		}
		if same {
			return nil
		}
		b, warn, err := s.backupData(project, op, data, path)
		if err != nil {
			return err
		}
		res.Backup = &b
		if warn != "" {
			res.Warnings = append(res.Warnings, warn)
		}
		if err := kubecfg.Save(result, path); err != nil {
			return err
		}
		res.Changed = true
		return nil
	})
}

// sameConfig reports whether a and b hold the same kubeconfig, ignoring how
// file references are spelled.
func sameConfig(a, b *api.Config) (bool, error) {
	ab, err := clientcmd.Write(*kubecfg.FlattenedCopy(a))
	if err != nil {
		return false, err
	}
	bb, err := clientcmd.Write(*kubecfg.FlattenedCopy(b))
	if err != nil {
		return false, err
	}
	return string(ab) == string(bb), nil
}

// PreviewImport lists the conflicts a merge of file into project would hit.
// A project that does not exist yet has none.
func (s *Store) PreviewImport(file, project string) ([]kubecfg.Conflict, error) {
	incoming, master, err := s.previewInputs(file, project)
	if err != nil || master == nil {
		return nil, err
	}
	return kubecfg.DetectConflicts(master, incoming), nil
}

// PreviewMerge runs the merge of file into project in memory and returns its
// report; nothing is written and no backup is taken. Like Import, conflicts
// that resolve to PolicyFail return an error wrapping kubecfg.ErrConflicts
// with the report filled in.
func (s *Store) PreviewMerge(file, project string, opts kubecfg.MergeOptions) (kubecfg.MergeReport, error) {
	incoming, master, err := s.previewInputs(file, project)
	if err != nil {
		return kubecfg.MergeReport{}, err
	}
	_, rep, err := kubecfg.Merge(master, incoming, opts)
	return rep, err
}

// previewInputs loads the flattened incoming file and the raw master of
// project; master is nil when the project does not exist yet.
func (s *Store) previewInputs(file, project string) (incoming, master *api.Config, err error) {
	if err := ValidateName(project); err != nil {
		return nil, nil, err
	}
	incoming, err = kubecfg.LoadFlattened(file)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", file, err)
	}
	exists, err := s.existingProject(project)
	if err != nil || !exists {
		return incoming, nil, err
	}
	path := s.ProjectPath(project)
	data, err := readProjectFile(path)
	if err != nil {
		return nil, nil, err
	}
	master, err = kubecfg.LoadBytes(data, path)
	if err != nil {
		return nil, nil, fmt.Errorf("project %s is not a valid kubeconfig: %w", project, err)
	}
	return incoming, master, nil
}

// RemoveContext deletes ctx from project's master file only, after a backup.
func (s *Store) RemoveContext(project, ctx string, keepOrphans bool) (kubecfg.DeleteReport, error) {
	if err := ValidateName(project); err != nil {
		return kubecfg.DeleteReport{}, err
	}
	exists, err := s.existingProject(project)
	if err != nil {
		return kubecfg.DeleteReport{}, err
	}
	if !exists {
		return kubecfg.DeleteReport{}, fmt.Errorf("%w: %s", ErrNotFound, project)
	}
	path := s.ProjectPath(project)
	var rep kubecfg.DeleteReport
	err = fsutil.WithLock(path, func() error {
		data, err := readProjectFile(path)
		if err != nil {
			return err
		}
		cfg, err := kubecfg.LoadBytes(data, path)
		if err != nil {
			return fmt.Errorf("project %s is not a valid kubeconfig: %w", project, err)
		}
		if rep, err = kubecfg.DeleteContext(cfg, ctx, keepOrphans); err != nil {
			return err
		}
		// DeleteReport has no room for a warning; a failed prune only means
		// old backups linger, and the new backup exists, so ignoring it is safe.
		if _, _, err := s.backupData(project, "rmctx", data, path); err != nil {
			return err
		}
		return kubecfg.Save(cfg, path)
	})
	if err != nil {
		return kubecfg.DeleteReport{}, err
	}
	return rep, nil
}

// Rename renames a project, its backup directory, the active link if it
// points at it, and the recorded previous project if it matches. Live shells
// are not checked; callers can warn via LiveShells.
func (s *Store) Rename(oldName, newName string) error {
	if err := ValidateName(oldName); err != nil {
		return err
	}
	if err := ValidateName(newName); err != nil {
		return err
	}
	exists, err := s.existingProject(oldName)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: %s", ErrNotFound, oldName)
	}
	if _, err := os.Lstat(s.ProjectPath(newName)); err == nil {
		return fmt.Errorf("%w: %s", ErrExists, newName)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	active, st, err := s.Active()
	if err != nil {
		return err
	}
	// The backup lands in the old project's directory and moves with it.
	// A failed prune only leaves old backups behind, so the warning is dropped.
	if _, _, err := s.backup(oldName, "rename", s.ProjectPath(oldName)); err != nil {
		return err
	}
	if err := os.Rename(s.ProjectPath(oldName), s.ProjectPath(newName)); err != nil {
		return fmt.Errorf("rename %s: %w", oldName, err)
	}
	if st == LinkOK && active == oldName {
		if err := s.swapLink(projectPrefix + newName); err != nil {
			_ = os.Rename(s.ProjectPath(newName), s.ProjectPath(oldName))
			return err
		}
	}
	if err := s.moveBackups(oldName, newName); err != nil {
		return fmt.Errorf("renamed project but could not move its backups: %w", err)
	}
	if prev, err := s.Previous(); err == nil && prev == oldName {
		if err := s.setPrevious(newName); err != nil {
			return fmt.Errorf("renamed project but could not update previous project: %w", err)
		}
	}
	return nil
}

// moveBackups moves <backups>/<old> to <backups>/<new>. If the destination
// already exists (leftovers of a deleted project of that name) the files are
// moved one by one; a file whose name collides stays in the old directory.
func (s *Store) moveBackups(oldName, newName string) error {
	oldDir := filepath.Join(s.Backups.Dir, oldName)
	newDir := filepath.Join(s.Backups.Dir, newName)
	if _, err := os.Lstat(oldDir); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := os.Lstat(newDir); errors.Is(err, fs.ErrNotExist) {
		return os.Rename(oldDir, newDir)
	} else if err != nil {
		return err
	}
	des, err := os.ReadDir(oldDir)
	if err != nil {
		return err
	}
	for _, de := range des {
		dst := filepath.Join(newDir, de.Name())
		if _, err := os.Lstat(dst); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := os.Rename(filepath.Join(oldDir, de.Name()), dst); err != nil {
			return err
		}
	}
	_ = os.Remove(oldDir) // only succeeds when empty
	return nil
}

// Delete removes a project after a final backup (op "delete"; the backups
// directory is kept). Deleting the active project needs force, which also
// removes the <root>/config link.
func (s *Store) Delete(name string, force bool) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	exists, err := s.existingProject(name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	active, st, err := s.Active()
	if err != nil {
		return err
	}
	isActive := st == LinkOK && active == name
	if isActive && !force {
		return fmt.Errorf("%w: %s (use force to delete it and deactivate)", ErrActive, name)
	}
	// Warning dropped: the final backup exists even if pruning failed.
	if _, _, err := s.backup(name, "delete", s.ProjectPath(name)); err != nil {
		return err
	}
	// File first: if it cannot be removed the link stays valid.
	if err := os.Remove(s.ProjectPath(name)); err != nil {
		return err
	}
	if isActive {
		if err := os.Remove(s.ConfigPath()); err != nil {
			return fmt.Errorf("deleted %s but could not remove the config link: %w", name, err)
		}
	}
	if prev, err := s.Previous(); err == nil && prev == name {
		if err := s.setPrevious(""); err != nil {
			return fmt.Errorf("deleted %s but could not clear previous project: %w", name, err)
		}
	}
	return nil
}

// Restore replaces project's master with backup id, after backing up the
// current file (op "prerestore") so the restore can be undone. The file is
// replaced in place, so the active link stays valid. A project that no longer
// exists (deleted) is recreated without a pre-restore backup.
func (s *Store) Restore(project, id string) error {
	if err := ValidateName(project); err != nil {
		return err
	}
	b, err := s.Backups.Get(project, id)
	if err != nil {
		return err
	}
	if _, err := kubecfg.Load(b.Path); err != nil {
		return fmt.Errorf("backup %s is not a valid kubeconfig: %w", id, err)
	}
	data, err := os.ReadFile(b.Path)
	if err != nil {
		return err
	}
	exists, err := s.existingProject(project)
	if err != nil {
		return err
	}
	path := s.ProjectPath(project)
	if !exists {
		if err := os.MkdirAll(s.Root, 0o700); err != nil {
			return err
		}
		return fsutil.WriteFileAtomic(path, data)
	}
	return fsutil.WithLock(path, func() error {
		// An empty current file is allowed here: restoring is how it is fixed.
		cur, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Warning dropped: the pre-restore backup exists even if pruning failed.
		if _, _, err := s.backupData(project, "prerestore", cur, path); err != nil {
			return err
		}
		return fsutil.WriteFileAtomic(path, data)
	})
}

// RestoreInitial writes the "_initial" backup id (the newest when id is empty)
// to a new project named as. It never overwrites an existing project and never
// touches <root>/config.
func (s *Store) RestoreInitial(id, as string) error {
	if err := ValidateName(as); err != nil {
		return err
	}
	if _, err := os.Lstat(s.ProjectPath(as)); err == nil {
		return fmt.Errorf("%w: %s", ErrExists, as)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	var b backup.Backup
	if id == "" {
		bs, err := s.Backups.List(backup.InitialProject)
		if err != nil {
			return err
		}
		if len(bs) == 0 {
			return fmt.Errorf("no backups of an original config: %w", fs.ErrNotExist)
		}
		b = bs[0]
	} else {
		var err error
		if b, err = s.Backups.Get(backup.InitialProject, id); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(b.Path)
	if err != nil {
		return err
	}
	if _, err := clientcmd.Load(data); err != nil {
		return fmt.Errorf("backup %s is not a valid kubeconfig: %w", b.ID, err)
	}
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(s.ProjectPath(as), data)
}

// Target is the file a ctx/ns command edits.
type Target struct {
	Path, Project string
	InShell       bool
}

// Target resolves which file ctx/ns commands edit: an explicit project flag
// always means that project's master; otherwise inside a kx shell the shell's
// copy; otherwise the active project's master.
func (s *Store) Target(projectFlag string, env Env) (Target, error) {
	if projectFlag != "" {
		if err := ValidateName(projectFlag); err != nil {
			return Target{}, err
		}
		exists, err := s.existingProject(projectFlag)
		if err != nil {
			return Target{}, err
		}
		if !exists {
			return Target{}, fmt.Errorf("%w: %s", ErrNotFound, projectFlag)
		}
		return Target{Path: s.ProjectPath(projectFlag), Project: projectFlag}, nil
	}
	if env.InShell {
		if err := s.checkShellKubeconfig(env.Kubeconfig); err != nil {
			return Target{}, err
		}
		return Target{Path: env.Kubeconfig, Project: env.Project, InShell: true}, nil
	}
	name, st, err := s.Active()
	if err != nil {
		return Target{}, err
	}
	switch st {
	case LinkOK:
		return Target{Path: s.ProjectPath(name), Project: name}, nil
	case LinkBroken:
		return Target{}, fmt.Errorf("%w: active project %s is missing (run kx use)", ErrNotFound, name)
	case LinkForeignFile:
		return Target{}, ErrForeignConfig
	case LinkForeignSymlink:
		return Target{}, ErrForeignSymlink
	}
	return Target{}, errors.New("no active project (run kx use)")
}

// SetContext makes ctx the current context of the target file. No backup:
// only the current-context field changes.
func (s *Store) SetContext(t Target, ctx string) error {
	return editTarget(t, func(cfg *api.Config) error { return kubecfg.SetCurrentContext(cfg, ctx) })
}

// SetNamespace sets the namespace of the target's current context. No backup:
// only one field changes and it is trivially reversible.
func (s *Store) SetNamespace(t Target, ns string) error {
	return editTarget(t, func(cfg *api.Config) error { return kubecfg.SetNamespace(cfg, ns) })
}

func editTarget(t Target, edit func(*api.Config) error) error {
	return fsutil.WithLock(t.Path, func() error {
		data, err := readProjectFile(t.Path)
		if err != nil {
			return fmt.Errorf("read %s: %w", t.Path, err)
		}
		cfg, err := kubecfg.LoadBytes(data, t.Path)
		if err != nil {
			return fmt.Errorf("read %s: %w", t.Path, err)
		}
		if err := edit(cfg); err != nil {
			return err
		}
		return kubecfg.Save(cfg, t.Path)
	})
}

// checkShellKubeconfig accepts only a single path directly inside ShellsDir():
// a KUBECONFIG changed inside the kx shell must never make ctx/ns edit some
// other file.
func (s *Store) checkShellKubeconfig(kc string) error {
	if kc != "" && !strings.ContainsRune(kc, filepath.ListSeparator) &&
		filepath.Clean(filepath.Dir(kc)) == filepath.Clean(s.ShellsDir()) {
		return nil
	}
	return fmt.Errorf("KUBECONFIG was changed inside the kx shell (%q is not its private copy under %s); restore it or exit the shell", kc, s.ShellsDir())
}

// Package kubecfg loads, saves and merges kubeconfig files.
package kubecfg

import (
	"fmt"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/nunoarmada/kxconfig/internal/fsutil"
)

// Load reads a kubeconfig file. Relative file references are left as-is and
// LocationOfOrigin is set on every entry.
func Load(path string) (*api.Config, error) {
	return clientcmd.LoadFromFile(path)
}

// LoadFlattened loads path, resolves relative file references against the
// file's directory and inlines referenced files as *-data fields, so the
// result is self-contained and comparable across files.
func LoadFlattened(path string) (*api.Config, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	if err := clientcmd.ResolveLocalPaths(cfg); err != nil {
		return nil, fmt.Errorf("resolve paths in %s: %w", path, err)
	}
	if err := api.FlattenConfig(cfg); err != nil {
		return nil, fmt.Errorf("flatten %s: %w", path, err)
	}
	return cfg, nil
}

// Save serialises cfg and writes it atomically with mode 0600, following a
// symlink at path.
func Save(cfg *api.Config, path string) error {
	data, err := clientcmd.Write(*cfg)
	if err != nil {
		return fmt.Errorf("serialise kubeconfig: %w", err)
	}
	return fsutil.WriteFileAtomic(path, data)
}

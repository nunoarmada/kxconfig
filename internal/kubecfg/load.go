// Package kubecfg loads, saves and merges kubeconfig files.
package kubecfg

import (
	"fmt"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/nunoarmada/kxconfig/v2/internal/fsutil"
)

// Load reads a kubeconfig file. Relative file references are left as-is and
// LocationOfOrigin is set on every entry.
func Load(path string) (*api.Config, error) {
	return clientcmd.LoadFromFile(path)
}

// LoadBytes parses kubeconfig data that was read from origin, setting
// LocationOfOrigin on every entry like Load does. Callers that read the file
// once (to back up exactly what they parse) use it instead of Load.
func LoadBytes(data []byte, origin string) (*api.Config, error) {
	cfg, err := clientcmd.Load(data)
	if err != nil {
		return nil, err
	}
	for _, c := range cfg.Clusters {
		c.LocationOfOrigin = origin
	}
	for _, u := range cfg.AuthInfos {
		u.LocationOfOrigin = origin
	}
	for _, c := range cfg.Contexts {
		c.LocationOfOrigin = origin
	}
	return cfg, nil
}

// FlattenedCopy returns a deep copy of cfg in which every cluster and user
// that can be flattened has its file references resolved and inlined. Entries
// that cannot be (no origin, or a missing file) stay as they are. It is meant
// for comparisons: the result is not what should be written back.
func FlattenedCopy(cfg *api.Config) *api.Config {
	out := cfg.DeepCopy()
	for name, c := range out.Clusters {
		out.Clusters[name] = flatCluster(c)
	}
	for name, u := range out.AuthInfos {
		out.AuthInfos[name] = flatUser(u)
	}
	return out
}

func flatCluster(c *api.Cluster) *api.Cluster {
	if c.LocationOfOrigin == "" {
		return c.DeepCopy()
	}
	tmp := &api.Config{Clusters: map[string]*api.Cluster{"c": c.DeepCopy()}}
	if clientcmd.ResolveLocalPaths(tmp) != nil || api.FlattenConfig(tmp) != nil {
		return c.DeepCopy()
	}
	return tmp.Clusters["c"]
}

func flatUser(u *api.AuthInfo) *api.AuthInfo {
	if u.LocationOfOrigin == "" {
		return u.DeepCopy()
	}
	tmp := &api.Config{AuthInfos: map[string]*api.AuthInfo{"u": u.DeepCopy()}}
	if clientcmd.ResolveLocalPaths(tmp) != nil || api.FlattenConfig(tmp) != nil {
		return u.DeepCopy()
	}
	return tmp.AuthInfos["u"]
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

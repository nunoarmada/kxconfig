package kubecfg

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"k8s.io/client-go/tools/clientcmd/api"
)

// DeleteReport describes what DeleteContext removed.
type DeleteReport struct {
	RemovedClusters, RemovedUsers []string
	// NewCurrent is the current context after the deletion ("" if none).
	NewCurrent string
}

// Orphans returns the clusters and users (sorted) that deleting ctx would
// leave unreferenced by any other context. It does not modify cfg.
func Orphans(cfg *api.Config, ctx string) (clusters, users []string) {
	c := cfg.Contexts[ctx]
	if c == nil {
		return nil, nil
	}
	usedCluster, usedUser := map[string]bool{}, map[string]bool{}
	for name, other := range cfg.Contexts {
		if name == ctx || other == nil {
			continue
		}
		usedCluster[other.Cluster] = true
		usedUser[other.AuthInfo] = true
	}
	if c.Cluster != "" && cfg.Clusters[c.Cluster] != nil && !usedCluster[c.Cluster] {
		clusters = []string{c.Cluster}
	}
	if c.AuthInfo != "" && cfg.AuthInfos[c.AuthInfo] != nil && !usedUser[c.AuthInfo] {
		users = []string{c.AuthInfo}
	}
	return clusters, users
}

// DeleteContext removes ctx and, unless keepOrphans is set, the cluster and
// user that only it referenced. If ctx was the current context, the current
// context becomes the first remaining one alphabetically, or "".
func DeleteContext(cfg *api.Config, ctx string, keepOrphans bool) (DeleteReport, error) {
	if cfg.Contexts[ctx] == nil {
		return DeleteReport{}, fmt.Errorf("context %q not found", ctx)
	}
	var rep DeleteReport
	if !keepOrphans {
		rep.RemovedClusters, rep.RemovedUsers = Orphans(cfg, ctx)
		for _, n := range rep.RemovedClusters {
			delete(cfg.Clusters, n)
		}
		for _, n := range rep.RemovedUsers {
			delete(cfg.AuthInfos, n)
		}
	}
	delete(cfg.Contexts, ctx)
	if cfg.CurrentContext == ctx {
		cfg.CurrentContext = ""
		if rest := names(cfg, KindContext); len(rest) > 0 {
			cfg.CurrentContext = rest[0]
		}
	}
	rep.NewCurrent = cfg.CurrentContext
	return rep, nil
}

// SetCurrentContext makes ctx the current context.
func SetCurrentContext(cfg *api.Config, ctx string) error {
	if cfg.Contexts[ctx] == nil {
		return fmt.Errorf("context %q not found", ctx)
	}
	cfg.CurrentContext = ctx
	return nil
}

var namespaceRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)

// ValidNamespace reports whether ns is a valid (non-empty) Kubernetes
// namespace name: a DNS-1123 label.
func ValidNamespace(ns string) bool { return namespaceRe.MatchString(ns) }

// SetNamespace sets the namespace of the current context.
func SetNamespace(cfg *api.Config, ns string) error {
	c := cfg.Contexts[cfg.CurrentContext]
	if cfg.CurrentContext == "" || c == nil {
		return errors.New("no current context")
	}
	c.Namespace = ns
	return nil
}

// DefaultNamespace is what kx writes into a current context that has none.
const DefaultNamespace = "default"

// FillNamespace sets DefaultNamespace on the current context when it has no
// namespace, and reports whether it changed anything. A namespace already set
// is kept, and a config without a usable current context is left alone.
func FillNamespace(cfg *api.Config) bool {
	c := cfg.Contexts[cfg.CurrentContext]
	if cfg.CurrentContext == "" || c == nil || c.Namespace != "" {
		return false
	}
	c.Namespace = DefaultNamespace
	return true
}

// ContextInfo is a display-oriented view of a context.
type ContextInfo struct {
	Name, Cluster, User, Namespace, Server, AuthKind string
	Current                                          bool
}

// Contexts lists all contexts sorted by name. Server is redacted (see
// RedactServer). References to missing clusters or users yield an empty
// Server and AuthKind "none".
func Contexts(cfg *api.Config) []ContextInfo {
	out := make([]ContextInfo, 0, len(cfg.Contexts))
	for _, name := range names(cfg, KindContext) {
		c := cfg.Contexts[name]
		if c == nil {
			continue
		}
		ci := ContextInfo{
			Name:      name,
			Cluster:   c.Cluster,
			User:      c.AuthInfo,
			Namespace: c.Namespace,
			AuthKind:  AuthKind(cfg.AuthInfos[c.AuthInfo]),
			Current:   name == cfg.CurrentContext,
		}
		if cl := cfg.Clusters[c.Cluster]; cl != nil && cl.Server != "" {
			ci.Server = RedactServer(cl.Server)
		}
		out = append(out, ci)
	}
	return out
}

// AuthKind returns the primary auth mechanism of a user: "exec: <command>",
// "auth-provider: <name>", "token", "client-cert", "basic" or "none". When
// several are configured the first in that order wins.
func AuthKind(a *api.AuthInfo) string {
	switch {
	case a == nil:
		return "none"
	case a.Exec != nil:
		return "exec: " + a.Exec.Command
	case a.AuthProvider != nil:
		return "auth-provider: " + a.AuthProvider.Name
	case a.Token != "" || a.TokenFile != "":
		return "token"
	case a.ClientCertificate != "" || len(a.ClientCertificateData) > 0 ||
		a.ClientKey != "" || len(a.ClientKeyData) > 0:
		return "client-cert"
	case a.Username != "" || a.Password != "":
		return "basic"
	}
	return "none"
}

// ValidEntityName reports why name cannot name a context, cluster or user,
// or "" when it can. kubeconfig allows almost anything; kx refuses what
// would be unreadable or break shells: empty names, surrounding spaces and
// control characters.
func ValidEntityName(name string) string {
	switch {
	case name == "":
		return "name is empty"
	case strings.TrimSpace(name) != name:
		return "name starts or ends with a space"
	case len(name) > 253:
		return "name is longer than 253 bytes"
	}
	for _, r := range name {
		if unicode.IsControl(r) || r == unicode.ReplacementChar {
			return "name has a control or invalid character"
		}
	}
	return ""
}

// RenameContext renames context oldName to newName, keeping it current if it
// was. Renaming to the same name is a no-op.
func RenameContext(cfg *api.Config, oldName, newName string) error {
	c := cfg.Contexts[oldName]
	if c == nil {
		return fmt.Errorf("context %q not found", oldName)
	}
	if oldName == newName {
		return nil
	}
	if msg := ValidEntityName(newName); msg != "" {
		return fmt.Errorf("invalid context name %q: %s", newName, msg)
	}
	if cfg.Contexts[newName] != nil {
		return fmt.Errorf("context %q already exists", newName)
	}
	delete(cfg.Contexts, oldName)
	cfg.Contexts[newName] = c
	if cfg.CurrentContext == oldName {
		cfg.CurrentContext = newName
	}
	return nil
}

// RenameCluster renames cluster oldName to newName and repoints every
// context that used it. It returns the contexts it repointed, sorted.
// Renaming to the same name is a no-op.
func RenameCluster(cfg *api.Config, oldName, newName string) ([]string, error) {
	cl := cfg.Clusters[oldName]
	if cl == nil {
		return nil, fmt.Errorf("cluster %q not found", oldName)
	}
	if oldName == newName {
		return nil, nil
	}
	if msg := ValidEntityName(newName); msg != "" {
		return nil, fmt.Errorf("invalid cluster name %q: %s", newName, msg)
	}
	if cfg.Clusters[newName] != nil {
		return nil, fmt.Errorf("cluster %q already exists", newName)
	}
	delete(cfg.Clusters, oldName)
	cfg.Clusters[newName] = cl
	var moved []string
	for _, name := range names(cfg, KindContext) {
		if c := cfg.Contexts[name]; c != nil && c.Cluster == oldName {
			c.Cluster = newName
			moved = append(moved, name)
		}
	}
	return moved, nil
}

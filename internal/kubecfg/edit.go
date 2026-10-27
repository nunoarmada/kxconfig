package kubecfg

import (
	"errors"
	"fmt"

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

// SetNamespace sets the namespace of the current context.
func SetNamespace(cfg *api.Config, ns string) error {
	c := cfg.Contexts[cfg.CurrentContext]
	if cfg.CurrentContext == "" || c == nil {
		return errors.New("no current context")
	}
	c.Namespace = ns
	return nil
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

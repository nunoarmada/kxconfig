package kubecfg

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/tools/clientcmd/api"
)

// Kind identifies the type of a kubeconfig entity.
type Kind string

// Entity kinds, in the order they are processed and reported.
const (
	KindCluster Kind = "cluster"
	KindUser    Kind = "user"
	KindContext Kind = "context"
)

var kindOrder = []Kind{KindCluster, KindUser, KindContext}

// EntityRef names one cluster, user or context.
type EntityRef struct {
	Kind Kind
	Name string
}

// Conflict is a same-name entity whose content differs. Existing and
// Incoming are safe one-line summaries that never contain secrets.
type Conflict struct {
	Ref      EntityRef
	Existing string
	Incoming string
}

// ConflictPolicy says what to do with a conflicting entity.
type ConflictPolicy string

// Conflict policies.
const (
	PolicyFail      ConflictPolicy = "fail"
	PolicyRename    ConflictPolicy = "rename"
	PolicyOverwrite ConflictPolicy = "overwrite"
	PolicySkip      ConflictPolicy = "skip"
)

// ParsePolicy converts a user-supplied string into a ConflictPolicy.
func ParsePolicy(s string) (ConflictPolicy, error) {
	switch p := ConflictPolicy(s); p {
	case PolicyFail, PolicyRename, PolicyOverwrite, PolicySkip:
		return p, nil
	}
	return "", fmt.Errorf("unknown conflict policy %q (want fail, rename, overwrite or skip)", s)
}

// MergeOptions selects the policy per conflicting entity. An empty Default
// means PolicyFail.
type MergeOptions struct {
	Default ConflictPolicy
	PerItem map[EntityRef]ConflictPolicy
}

// MergeReport describes what Merge did. All slices are ordered by kind
// (cluster, user, context) then name. Conflicts lists only the conflicts
// that resolved to PolicyFail.
type MergeReport struct {
	Added, Unchanged, Overwritten, Skipped []EntityRef
	Renamed                                map[EntityRef]string
	Conflicts                              []Conflict
}

// ErrConflicts is returned by Merge when at least one conflict resolves to
// PolicyFail.
var ErrConflicts = errors.New("conflicting entries")

// DetectConflicts lists same-name entities whose content differs between
// master and incoming (ordered by kind, then name).
func DetectConflicts(master, incoming *api.Config) []Conflict {
	master, incoming = orEmpty(master), orEmpty(incoming)
	var out []Conflict
	for _, k := range kindOrder {
		for _, name := range names(incoming, k) {
			m, ok := entity(master, k, name)
			if !ok {
				continue
			}
			i, _ := entity(incoming, k, name)
			if !equal(m, i) {
				r := EntityRef{k, name}
				out = append(out, Conflict{Ref: r, Existing: Summarize(r, master), Incoming: Summarize(r, incoming)})
			}
		}
	}
	return out
}

// Merge combines incoming into a copy of master and returns it. Neither input
// is mutated and master's CurrentContext is kept. If any conflict resolves to
// PolicyFail it returns a nil config, the report (with Conflicts) and
// ErrConflicts.
func Merge(master, incoming *api.Config, opts MergeOptions) (*api.Config, MergeReport, error) {
	master, incoming = orEmpty(master), orEmpty(incoming)
	if err := validatePolicies(opts); err != nil {
		return nil, MergeReport{}, err
	}
	result := master.DeepCopy()
	rep := MergeReport{Renamed: map[EntityRef]string{}}

	used := map[Kind]map[string]bool{}
	for _, k := range kindOrder {
		used[k] = map[string]bool{}
		for _, n := range names(master, k) {
			used[k][n] = true
		}
		for _, n := range names(incoming, k) {
			used[k][n] = true
		}
	}

	for _, k := range kindOrder {
		for _, name := range names(incoming, k) {
			r := EntityRef{k, name}
			inc, _ := entity(incoming, k, name)
			if ctx, ok := inc.(*api.Context); ok {
				rewriteContext(ctx, rep.Renamed)
			}
			cur, exists := entity(master, k, name)
			switch {
			case !exists:
				put(result, k, name, inc)
				rep.Added = append(rep.Added, r)
			case equal(cur, inc):
				rep.Unchanged = append(rep.Unchanged, r)
			default:
				switch policyFor(opts, r) {
				case PolicyOverwrite:
					put(result, k, name, inc)
					rep.Overwritten = append(rep.Overwritten, r)
				case PolicySkip:
					rep.Skipped = append(rep.Skipped, r)
				case PolicyRename:
					nn := freeName(used[k], name)
					used[k][nn] = true
					put(result, k, nn, inc)
					rep.Renamed[r] = nn
				default:
					rep.Conflicts = append(rep.Conflicts, Conflict{
						Ref: r, Existing: Summarize(r, master), Incoming: Summarize(r, incoming),
					})
				}
			}
		}
	}
	if len(rep.Conflicts) > 0 {
		return nil, rep, ErrConflicts
	}
	return result, rep, nil
}

// Summarize returns a safe one-line description of an entity: the server URL
// (without credentials) for a cluster, the kinds of auth configured for a
// user, and "cluster/user namespace" for a context. It never includes secret
// values.
func Summarize(ref EntityRef, cfg *api.Config) string {
	e, ok := entity(orEmpty(cfg), ref.Kind, ref.Name)
	if !ok {
		return "(missing)"
	}
	switch v := e.(type) {
	case *api.Cluster:
		return redactServer(v.Server)
	case *api.AuthInfo:
		return authKinds(v)
	case *api.Context:
		s := v.Cluster + "/" + v.AuthInfo
		if v.Namespace != "" {
			s += " " + v.Namespace
		}
		return s
	}
	return ""
}

func authKinds(u *api.AuthInfo) string {
	var kinds []string
	if u.Exec != nil {
		kinds = append(kinds, "exec")
	}
	if u.AuthProvider != nil {
		kinds = append(kinds, "auth-provider")
	}
	if u.Token != "" || u.TokenFile != "" {
		kinds = append(kinds, "token")
	}
	if u.Username != "" || u.Password != "" {
		kinds = append(kinds, "basic auth")
	}
	if u.ClientCertificate != "" || len(u.ClientCertificateData) > 0 ||
		u.ClientKey != "" || len(u.ClientKeyData) > 0 {
		kinds = append(kinds, "client certificate")
	}
	if len(kinds) == 0 {
		return "none"
	}
	return strings.Join(kinds, ", ")
}

func redactServer(server string) string {
	u, err := url.Parse(server)
	if err != nil {
		return "(unparseable server)"
	}
	u.User = nil
	return u.String()
}

func orEmpty(c *api.Config) *api.Config {
	if c == nil {
		return api.NewConfig()
	}
	return c
}

func validatePolicies(opts MergeOptions) error {
	check := func(p ConflictPolicy) error {
		if p == "" {
			return nil
		}
		_, err := ParsePolicy(string(p))
		return err
	}
	if err := check(opts.Default); err != nil {
		return err
	}
	for _, p := range opts.PerItem {
		if err := check(p); err != nil {
			return err
		}
	}
	return nil
}

func policyFor(opts MergeOptions, r EntityRef) ConflictPolicy {
	if p, ok := opts.PerItem[r]; ok && p != "" {
		return p
	}
	if opts.Default != "" {
		return opts.Default
	}
	return PolicyFail
}

// freeName returns the first of name-2, name-3, ... not present in used.
func freeName(used map[string]bool, name string) string {
	for i := 2; ; i++ {
		if c := fmt.Sprintf("%s-%d", name, i); !used[c] {
			return c
		}
	}
}

func rewriteContext(ctx *api.Context, renamed map[EntityRef]string) {
	if n, ok := renamed[EntityRef{KindCluster, ctx.Cluster}]; ok {
		ctx.Cluster = n
	}
	if n, ok := renamed[EntityRef{KindUser, ctx.AuthInfo}]; ok {
		ctx.AuthInfo = n
	}
}

func names(c *api.Config, k Kind) []string {
	var out []string
	switch k {
	case KindCluster:
		for n := range c.Clusters {
			out = append(out, n)
		}
	case KindUser:
		for n := range c.AuthInfos {
			out = append(out, n)
		}
	case KindContext:
		for n := range c.Contexts {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// entity returns a deep copy of the named entity with LocationOfOrigin
// cleared, so copies are comparable and safe to insert elsewhere.
func entity(c *api.Config, k Kind, name string) (any, bool) {
	switch k {
	case KindCluster:
		if v := c.Clusters[name]; v != nil {
			v = v.DeepCopy()
			v.LocationOfOrigin = ""
			return v, true
		}
	case KindUser:
		if v := c.AuthInfos[name]; v != nil {
			v = v.DeepCopy()
			v.LocationOfOrigin = ""
			return v, true
		}
	case KindContext:
		if v := c.Contexts[name]; v != nil {
			v = v.DeepCopy()
			v.LocationOfOrigin = ""
			return v, true
		}
	}
	return nil, false
}

func put(c *api.Config, k Kind, name string, e any) {
	switch v := e.(type) {
	case *api.Cluster:
		c.Clusters[name] = v
	case *api.AuthInfo:
		c.AuthInfos[name] = v
	case *api.Context:
		c.Contexts[name] = v
	}
}

func equal(a, b any) bool {
	return apiequality.Semantic.DeepEqual(a, b)
}

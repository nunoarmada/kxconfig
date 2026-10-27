// Command kx manages kubeconfig projects.
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"golang.org/x/term"

	"github.com/nunoarmada/kxconfig/v2/internal/cli"
	"github.com/nunoarmada/kxconfig/v2/internal/store"
	"github.com/nunoarmada/kxconfig/v2/internal/tui"
)

// version is set at release time with -ldflags "-X main.version=...".
var version = "dev"

// resolveVersion returns v, or for an unstamped build ("dev") the module
// version recorded by "go install ...@vX.Y.Z", when there is one.
func resolveVersion(v string, info *debug.BuildInfo) string {
	if v == "dev" && info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return v
}

func main() {
	root, err := store.DefaultRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "kx: %v\n", err)
		os.Exit(1)
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		version = resolveVersion(version, info)
	}
	st, env := store.New(root), store.EnvFromOS()
	d := cli.Deps{
		Store:   st,
		Env:     env,
		In:      os.Stdin,
		Out:     os.Stdout,
		Err:     os.Stderr,
		IsTTY:   term.IsTerminal(int(os.Stdin.Fd())),
		Version: version,
		RunTUI:  func() error { return tui.Run(st, env) },
	}
	os.Exit(cli.Run(d, os.Args[1:]))
}

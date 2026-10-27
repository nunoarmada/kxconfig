// Command kx manages kubeconfig projects.
package main

import (
	"fmt"
	"os"

	"golang.org/x/term"

	"github.com/nunoarmada/kxconfig/internal/cli"
	"github.com/nunoarmada/kxconfig/internal/store"
	"github.com/nunoarmada/kxconfig/internal/tui"
)

var version = "dev"

func main() {
	root, err := store.DefaultRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "kx: %v\n", err)
		os.Exit(1)
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

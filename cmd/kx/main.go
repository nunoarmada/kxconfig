// Command kx manages kubeconfig projects.
package main

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/term"

	"github.com/nunoarmada/kxconfig/internal/cli"
	"github.com/nunoarmada/kxconfig/internal/store"
)

var version = "dev"

func main() {
	root, err := store.DefaultRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "kx: %v\n", err)
		os.Exit(1)
	}
	d := cli.Deps{
		Store:   store.New(root),
		Env:     store.EnvFromOS(),
		In:      os.Stdin,
		Out:     os.Stdout,
		Err:     os.Stderr,
		IsTTY:   term.IsTerminal(int(os.Stdin.Fd())),
		Version: version,
		// Replaced by the real TUI in a later task.
		RunTUI: func() error { return errors.New("interactive UI not built yet") },
	}
	os.Exit(cli.Run(d, os.Args[1:]))
}

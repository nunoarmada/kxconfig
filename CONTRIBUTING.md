# Contributing

Thank you for considering a contribution to `kx`.

## Reporting bugs and suggesting features

Open an [issue](https://github.com/nunoarmada/kxconfig/issues) after checking that none exists yet. For a bug, include the `kx version` output, your OS, the steps to reproduce, and what you expected. Never paste kubeconfig contents with tokens or certificates.

## Development setup

You need the Go toolchain version in `go.mod` (the `toolchain` line; Go downloads it for you if your `go` is newer than 1.21) and [golangci-lint](https://golangci-lint.run/) v2.

```bash
git clone https://github.com/nunoarmada/kxconfig.git
cd kxconfig
go build -o kx ./cmd/kx
```

Layout: `cmd/kx` is the entry point, `internal/cli` the cobra commands, `internal/tui` the Bubble Tea UI, and `internal/store`, `internal/kubecfg`, `internal/backup` the logic they share.

## Test and lint

Run these before opening a pull request; CI runs the same on Linux and macOS.

```bash
go vet ./...
go test -race ./...
golangci-lint run ./...
go build ./cmd/kx
```

Tests that touch the file system use a temporary `KX_HOME`; never point a test at your real `~/.kube`. Write a failing test first for new behaviour and bug fixes.

To try a change safely by hand:

```bash
export KX_HOME=$(mktemp -d)
go run ./cmd/kx import some-kubeconfig.yaml -p demo
go run ./cmd/kx
```

The release configuration can be checked with `goreleaser check` (exit code 2 is expected: it reports the deprecated `brews` section) and `goreleaser release --snapshot --clean`.

## Pull requests

1. Branch from `main` (`feat/...`, `fix/...`).
2. Keep the change focused; do not mix refactors with behaviour changes.
3. Update the README when commands, keys or behaviour change.
4. Do not edit `CHANGELOG.md`; release-please writes it.

## Commit messages

Commits follow [Conventional Commits](https://www.conventionalcommits.org/); release-please builds the changelog and picks the version from them.

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

Types: `feat` (minor bump), `fix` (patch bump), `docs`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`. A `!` after the type or a `BREAKING CHANGE:` footer means a major bump.

```
feat(cli): add --output json to kx backups
fix(kubecfg): keep extensions when renaming a cluster
docs(readme): document KX_ASCII
```

## Releases

Merging the release PR that release-please opens on `main` creates the `vX.Y.Z` tag and GitHub release; the same workflow then runs GoReleaser to attach the binaries and update the Homebrew formula. It needs the `HOMEBREW_TAP_TOKEN` repository secret.

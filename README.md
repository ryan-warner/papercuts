# papercuts

`papercuts` records avoidable friction that AI agents and developers encounter
while working in software projects. Entries are stored in one user-level SQLite
database and grouped by Git repository root.

```sh
papercuts add "The fixture generator is undocumented"
papercuts list
papercuts stats
papercuts fix > PAPERCUTS_FIX.md
papercuts review
```

## Install a release

Download the archive for your operating system and processor from the
[Releases page](https://github.com/ryan-warner/papercuts/releases).
This repository is private, so you must sign in to GitHub with ForeFlight
access before you download a release.

On macOS or Linux, extract the archive and install the executable in a
directory on `PATH`:

```sh
tar -xzf papercuts_*.tar.gz
install papercuts "$HOME/.local/bin/papercuts"
papercuts version
```

On Windows, extract the ZIP file. Move `papercuts.exe` to a directory on
`PATH`, and then run:

```powershell
papercuts version
```

To install from source, use Go 1.25 or later:

```sh
GOPRIVATE=github.com/ryan-warner/* \
  go install github.com/ryan-warner/papercuts/cmd/papercuts@latest
```

## Agent usage

Run `papercuts agent-instructions` to print a short instruction suitable for an
`AGENTS.md` file. Commands group a primary Git checkout and all its linked
worktrees as one project. Each entry keeps the worktree and directory where it
was recorded. Commands fall back to the current directory outside Git. Use
`--project PATH` to set an exact project scope instead.

Entries made by an older release are grouped automatically while their
worktrees are still registered with Git. If a recorded worktree was deleted,
merge its old project path explicitly:

```sh
papercuts projects merge /path/to/deleted-worktree --into /path/to/primary-checkout
```

Machine callers should request `--json` or `--jsonl`. Diagnostics are written to
stderr and structured results to stdout.

## Storage

The database is stored in the platform user-data directory. Set
`PAPERCUTS_HOME` to override its directory, or pass `--database PATH` for a
single invocation.

## Development

```sh
go test ./...
go build ./cmd/papercuts
```

## Release

Releases use semantic version tags. To publish an early release after the
`main` branch is ready, create and push a tag such as `v0.1.0`:

```sh
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

GitHub Actions builds archives for macOS, Linux, and Windows on AMD64 and
ARM64. It creates the GitHub release, release notes, and `checksums.txt`.

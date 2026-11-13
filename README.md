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

See the [command guide](docs/commands.md) for the complete reference.

## Install

Install the latest release:

```sh
curl -fsSL https://raw.githubusercontent.com/ryan-warner/papercuts/main/install.sh | sh
```

The script detects the operating system and processor, verifies the archive
against `checksums.txt`, and installs the executable in `$HOME/.local/bin`.
Set `PAPERCUTS_INSTALL_DIR` or pass a directory as the first argument to
install elsewhere. The installer then offers to append agent guidance to the
Codex, Claude Code, and OpenCode instruction files it detects.

To install manually, download the archive for your operating system and
processor from the [Releases page](https://github.com/ryan-warner/papercuts/releases).
On macOS or Linux, extract it and copy `papercuts` to a directory on `PATH`.
On Windows, extract the ZIP file and move `papercuts.exe` to a directory on
`PATH`.

To install from source, use Go 1.25 or later:

```sh
go install github.com/ryan-warner/papercuts/cmd/papercuts@latest
```

## Agent usage

Because papercuts applies across projects, put its instructions in your global
agent guidance. Run the commands once for each agent you use:

```sh
# Codex
papercuts agent-instructions >> "$HOME/.codex/AGENTS.md"

# Claude Code
papercuts agent-instructions >> "$HOME/.claude/CLAUDE.md"
```

Use a repository-level `AGENTS.md` or `CLAUDE.md` only when the guidance should
apply to that repository. Commands group a primary Git checkout and all its
linked worktrees as one project. Each entry keeps the worktree and directory
where it was recorded. Commands fall back to the current directory outside
Git. Use `--project PATH` to set an exact project scope instead.

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

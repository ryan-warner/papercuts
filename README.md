# papercuts

`papercuts` records avoidable friction that AI agents and developers encounter
while working in software projects. Entries are stored in one user-level SQLite
database and grouped by Git repository root.

Record a papercut when you find a recurring problem that a maintainer can fix.
Use `list` or `review` to revisit open entries, `resolve` when the problem is
fixed, and `reopen` if it returns. The `fix` command turns open entries into a
maintenance prompt that you can give to an agent.

```sh
papercuts add "The fixture generator is undocumented"
papercuts list
papercuts stats
papercuts fix > PAPERCUTS_FIX.md
papercuts review
```

## Commands

| Command | Use |
| --- | --- |
| `papercuts` | Show the current project's open and resolved counts. |
| `papercuts add "<observation>"` | Record a concise description of avoidable friction. Use `--stdin` for piped input. |
| `papercuts list` | List open entries for the current project. Flags can include resolved entries or entries from all projects. |
| `papercuts show <id>` | Show one entry and its recorded project context. |
| `papercuts resolve <id...>` | Mark one or more entries as resolved. |
| `papercuts reopen <id...>` | Return one or more resolved entries to the open list. |
| `papercuts review` | Interactively review open entries and resolve selected items. |
| `papercuts fix` | Generate an agent-ready maintenance prompt from open entries. |
| `papercuts stats` | Show totals, the oldest open entry, and average resolution time. |
| `papercuts projects` | List every project with recorded entries and its totals. |
| `papercuts doctor` | Show the detected repository, worktree, working directory, and database path. |
| `papercuts agent-instructions` | Print reusable guidance that tells an agent when to record a papercut. |
| `papercuts completion <shell>` | Generate shell completion configuration. |
| `papercuts version` | Print the installed version. |

Use the help flag to see all commands or the flags for one command:

```sh
papercuts --help
papercuts <command> --help
```

## Install a release

Download the archive for your operating system and processor from the
[Releases page](https://github.com/ryan-warner/papercuts/releases).

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

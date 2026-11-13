# Command guide

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
| `papercuts setup-agents` | Add papercuts guidance to the agent instruction files detected on the machine. |
| `papercuts completion <shell>` | Generate shell completion configuration. |
| `papercuts version` | Print the installed version. |

Use the help flag to see all commands or the flags for one command:

```sh
papercuts --help
papercuts <command> --help
```

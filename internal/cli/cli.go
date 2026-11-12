package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/ryan-warner/papercuts/internal/fix"
	"github.com/ryan-warner/papercuts/internal/project"
	"github.com/ryan-warner/papercuts/internal/store"
	"github.com/spf13/cobra"
)

type appError struct {
	code int
	err  error
}

func (e *appError) Error() string { return e.err.Error() }
func (e *appError) Unwrap() error { return e.err }

func ExitCode(err error) int {
	var appErr *appError
	if errors.As(err, &appErr) {
		return appErr.code
	}
	return 1
}

type app struct {
	version         string
	projectOverride string
	jsonOutput      bool
	noColor         bool
	noInput         bool
	dbPath          string
	out             io.Writer
	errOut          io.Writer
	in              io.Reader
}

func New(version string) *cobra.Command {
	a := &app{version: version, out: os.Stdout, errOut: os.Stderr, in: os.Stdin}
	root := &cobra.Command{
		Use:           "papercuts",
		Short:         "Log and resolve friction encountered while building software",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE:          a.runSummary,
	}
	root.SetOut(a.out)
	root.SetErr(a.errOut)
	root.PersistentFlags().StringVar(&a.projectOverride, "project", "", "override the detected project directory")
	root.PersistentFlags().BoolVar(&a.jsonOutput, "json", false, "write stable JSON to stdout")
	root.PersistentFlags().BoolVar(&a.noColor, "no-color", false, "disable terminal colors")
	root.PersistentFlags().BoolVar(&a.noInput, "no-input", false, "disable interactive input")
	root.PersistentFlags().StringVar(&a.dbPath, "database", "", "override the database path")

	root.AddCommand(
		a.addCommand(), a.listCommand(), a.showCommand(),
		a.stateCommand("resolve", true), a.stateCommand("reopen", false),
		a.reviewCommand(), a.statsCommand(), a.projectsCommand(),
		a.fixCommand(), a.instructionsCommand(), a.doctorCommand(),
	)
	root.AddCommand(&cobra.Command{
		Use: "version", Short: "Print the version", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if a.jsonOutput {
				return writeJSON(a.out, map[string]string{"version": a.version})
			}
			fmt.Fprintln(a.out, a.version)
			return nil
		},
	})
	return root
}

func (a *app) context() (project.Context, error) {
	wd, err := os.Getwd()
	if err != nil {
		return project.Context{}, err
	}
	ctx, err := project.Resolve(wd, a.projectOverride)
	if err != nil {
		return project.Context{}, &appError{code: 2, err: fmt.Errorf("resolve project: %w", err)}
	}
	return ctx, nil
}

func (a *app) store() (*store.Store, error) {
	path := a.dbPath
	if path == "" {
		var err error
		path, err = store.DefaultPath()
		if err != nil {
			return nil, err
		}
	}
	return store.Open(path)
}

func (a *app) scopedStore(commandContext context.Context, projectContext project.Context) (*store.Store, error) {
	s, err := a.store()
	if err != nil {
		return nil, err
	}
	if err := s.AssociateRepository(commandContext, projectContext.RepositoryID, projectContext.ProjectPath, projectContext.LegacyProjectPaths); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (a *app) runSummary(cmd *cobra.Command, _ []string) error {
	ctx, err := a.context()
	if err != nil {
		return err
	}
	s, err := a.scopedStore(cmd.Context(), ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	stats, err := s.Stats(cmd.Context(), ctx.RepositoryID, ctx.ProjectPath, false)
	if err != nil {
		return err
	}
	if a.jsonOutput {
		return writeJSON(a.out, stats)
	}
	fmt.Fprintf(a.out, "papercuts — %s\n\n  %d open  %d resolved\n\n", filepath.Base(ctx.ProjectPath), stats.Open, stats.Resolved)
	fmt.Fprintln(a.out, "Use `papercuts add <observation>` to log friction or `papercuts review` to clean it up.")
	return nil
}

func (a *app) addCommand() *cobra.Command {
	var fromStdin bool
	cmd := &cobra.Command{
		Use:   "add [observation]",
		Short: "Record a new papercut",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := strings.TrimSpace(strings.Join(args, " "))
			if fromStdin || (body == "" && !isTerminalReader(a.in)) {
				data, err := io.ReadAll(io.LimitReader(a.in, 1024*1024+1))
				if err != nil {
					return err
				}
				if len(data) > 1024*1024 {
					return &appError{code: 2, err: errors.New("observation exceeds 1 MiB")}
				}
				body = strings.TrimSpace(string(data))
			}
			if body == "" {
				return &appError{code: 2, err: errors.New("observation cannot be empty")}
			}
			ctx, err := a.context()
			if err != nil {
				return err
			}
			s, err := a.scopedStore(cmd.Context(), ctx)
			if err != nil {
				return err
			}
			defer s.Close()
			entry, err := s.Add(cmd.Context(), ctx.RepositoryID, ctx.ProjectPath, ctx.WorktreePath, ctx.WorkingDir, body)
			if err != nil {
				return err
			}
			if a.jsonOutput {
				return writeJSON(a.out, entry)
			}
			fmt.Fprintf(a.out, "Recorded papercut #%d in %s\n", entry.ID, filepath.Base(ctx.ProjectPath))
			return nil
		},
	}
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "read the observation from stdin")
	return cmd
}

func (a *app) listCommand() *cobra.Command {
	var allProjects, includeResolved, onlyResolved, jsonLines bool
	var limit int
	cmd := &cobra.Command{
		Use: "list", Short: "List recorded papercuts", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if includeResolved && onlyResolved {
				return &appError{code: 2, err: errors.New("--resolved and --only-resolved cannot be used together")}
			}
			ctx, err := a.context()
			if err != nil {
				return err
			}
			state := "open"
			if includeResolved {
				state = "all"
			} else if onlyResolved {
				state = "resolved"
			}
			s, err := a.scopedStore(cmd.Context(), ctx)
			if err != nil {
				return err
			}
			defer s.Close()
			entries, err := s.List(cmd.Context(), store.ListOptions{RepositoryID: ctx.RepositoryID, AllProjects: allProjects, State: state, Limit: limit})
			if err != nil {
				return err
			}
			if jsonLines {
				encoder := json.NewEncoder(a.out)
				encoder.SetEscapeHTML(false)
				for _, entry := range entries {
					if err := encoder.Encode(entry); err != nil {
						return err
					}
				}
				return nil
			}
			if a.jsonOutput {
				return writeJSON(a.out, entries)
			}
			writeEntries(a.out, entries, allProjects)
			return nil
		},
	}
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "include entries from every project")
	cmd.Flags().BoolVar(&includeResolved, "resolved", false, "include resolved entries")
	cmd.Flags().BoolVar(&onlyResolved, "only-resolved", false, "show only resolved entries")
	cmd.Flags().BoolVar(&jsonLines, "jsonl", false, "write one JSON object per line")
	cmd.Flags().IntVar(&limit, "limit", 100, "maximum number of entries")
	return cmd
}

func (a *app) showCommand() *cobra.Command {
	return &cobra.Command{
		Use: "show <id>", Short: "Show a papercut", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			s, err := a.store()
			if err != nil {
				return err
			}
			defer s.Close()
			entry, err := s.Get(cmd.Context(), id)
			if errors.Is(err, store.ErrNotFound) {
				return &appError{code: 3, err: fmt.Errorf("papercut #%d not found", id)}
			}
			if err != nil {
				return err
			}
			if a.jsonOutput {
				return writeJSON(a.out, entry)
			}
			status := "open"
			if entry.ResolvedAt != nil {
				status = "resolved " + *entry.ResolvedAt
			}
			fmt.Fprintf(a.out, "#%d · %s\nProject: %s\nDirectory: %s\nCreated: %s\n\n%s\n", entry.ID, status, entry.ProjectPath, entry.WorkingDir, entry.CreatedAt, entry.Body)
			return nil
		},
	}
}

func (a *app) stateCommand(name string, resolved bool) *cobra.Command {
	return &cobra.Command{
		Use: name + " <id...>", Short: strings.ToUpper(name[:1]) + name[1:] + " one or more papercuts", Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := make([]int64, 0, len(args))
			for _, arg := range args {
				id, err := parseID(arg)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			s, err := a.store()
			if err != nil {
				return err
			}
			defer s.Close()
			affected, err := s.SetResolved(cmd.Context(), ids, resolved)
			if err != nil {
				return err
			}
			result := map[string]any{"action": name, "requested_ids": ids, "affected": affected}
			if a.jsonOutput {
				return writeJSON(a.out, result)
			}
			fmt.Fprintf(a.out, "%s %d papercut(s)\n", pastTense(name), affected)
			return nil
		},
	}
}

func (a *app) reviewCommand() *cobra.Command {
	return &cobra.Command{
		Use: "review", Short: "Interactively review and resolve open papercuts", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if a.jsonOutput || a.noInput || !isTerminalReader(a.in) {
				return &appError{code: 2, err: errors.New("review requires an interactive terminal")}
			}
			ctx, err := a.context()
			if err != nil {
				return err
			}
			s, err := a.scopedStore(cmd.Context(), ctx)
			if err != nil {
				return err
			}
			defer s.Close()
			entries, err := s.List(cmd.Context(), store.ListOptions{RepositoryID: ctx.RepositoryID, State: "open", Limit: 100})
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Fprintln(a.out, "No open papercuts.")
				return nil
			}
			options := make([]huh.Option[int64], 0, len(entries))
			for _, entry := range entries {
				options = append(options, huh.NewOption(fmt.Sprintf("#%d  %s", entry.ID, truncate(oneLine(entry.Body), 72)), entry.ID))
			}
			var selected []int64
			form := huh.NewForm(huh.NewGroup(huh.NewMultiSelect[int64]().Title("Resolve papercuts").Description("Select items whose underlying friction has been addressed.").Options(options...).Value(&selected)))
			if err := form.Run(); err != nil {
				return err
			}
			if len(selected) == 0 {
				fmt.Fprintln(a.out, "Nothing resolved.")
				return nil
			}
			n, err := s.SetResolved(cmd.Context(), selected, true)
			if err != nil {
				return err
			}
			fmt.Fprintf(a.out, "Resolved %d papercut(s)\n", n)
			return nil
		},
	}
}

func (a *app) statsCommand() *cobra.Command {
	var allProjects bool
	cmd := &cobra.Command{
		Use: "stats", Short: "Show papercut statistics", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, err := a.context()
			if err != nil {
				return err
			}
			s, err := a.scopedStore(cmd.Context(), ctx)
			if err != nil {
				return err
			}
			defer s.Close()
			stats, err := s.Stats(cmd.Context(), ctx.RepositoryID, ctx.ProjectPath, allProjects)
			if err != nil {
				return err
			}
			if a.jsonOutput {
				return writeJSON(a.out, stats)
			}
			scope := filepath.Base(ctx.ProjectPath)
			if allProjects {
				scope = "all projects"
			}
			fmt.Fprintf(a.out, "%s\n\nOpen:       %d\nResolved:   %d\nTotal:      %d\n", scope, stats.Open, stats.Resolved, stats.Total)
			if stats.OldestOpenAt != nil {
				fmt.Fprintf(a.out, "Oldest open: %s\n", friendlyTime(*stats.OldestOpenAt))
			}
			if stats.Resolved > 0 {
				fmt.Fprintf(a.out, "Avg resolve: %.1f hours\n", stats.AverageResolveHr)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "aggregate every project")
	return cmd
}

func (a *app) projectsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "projects", Short: "List projects with recorded papercuts", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, err := a.context()
			if err != nil {
				return err
			}
			s, err := a.scopedStore(cmd.Context(), ctx)
			if err != nil {
				return err
			}
			defer s.Close()
			projects, err := s.Projects(cmd.Context())
			if err != nil {
				return err
			}
			if a.jsonOutput {
				return writeJSON(a.out, projects)
			}
			w := tabwriter.NewWriter(a.out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "PROJECT\tOPEN\tRESOLVED\tLAST SEEN")
			for _, item := range projects {
				last := "—"
				if item.LastSeenAt != nil {
					last = friendlyTime(*item.LastSeenAt)
				}
				fmt.Fprintf(w, "%s\t%d\t%d\t%s\n", item.ProjectPath, item.Open, item.Resolved, last)
			}
			return w.Flush()
		},
	}
	cmd.AddCommand(a.projectsMergeCommand())
	return cmd
}

func (a *app) projectsMergeCommand() *cobra.Command {
	var destination string
	cmd := &cobra.Command{
		Use: "merge <source-project>", Short: "Merge a legacy project into a repository", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if destination == "" {
				return &appError{code: 2, err: errors.New("--into is required")}
			}
			target, err := project.Resolve(destination, "")
			if err != nil {
				return &appError{code: 2, err: fmt.Errorf("resolve destination project: %w", err)}
			}
			source, err := canonicalMissingPath(args[0])
			if err != nil {
				return err
			}
			s, err := a.scopedStore(cmd.Context(), target)
			if err != nil {
				return err
			}
			defer s.Close()
			affected, err := s.MergeProject(cmd.Context(), source, target.RepositoryID, target.ProjectPath)
			if err != nil {
				return err
			}
			result := map[string]any{"source_project": source, "repository_id": target.RepositoryID, "project_path": target.ProjectPath, "affected": affected}
			if a.jsonOutput {
				return writeJSON(a.out, result)
			}
			fmt.Fprintf(a.out, "Merged %d papercut(s) into %s\n", affected, target.ProjectPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&destination, "into", "", "primary checkout or linked worktree receiving the entries")
	return cmd
}

func canonicalMissingPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := filepath.Clean(abs)
	var missing []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", resolveErr
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func (a *app) fixCommand() *cobra.Command {
	var allProjects bool
	var limit int
	cmd := &cobra.Command{
		Use: "fix", Short: "Generate a maintenance prompt for open papercuts", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, err := a.context()
			if err != nil {
				return err
			}
			s, err := a.scopedStore(cmd.Context(), ctx)
			if err != nil {
				return err
			}
			defer s.Close()
			entries, err := s.List(cmd.Context(), store.ListOptions{RepositoryID: ctx.RepositoryID, AllProjects: allProjects, State: "open", Limit: limit})
			if err != nil {
				return err
			}
			scope := "project `" + ctx.ProjectPath + "`"
			if allProjects {
				scope = "all recorded projects"
			}
			prompt := fix.Build(scope, entries)
			if a.jsonOutput {
				return writeJSON(a.out, map[string]any{"project_path": ctx.ProjectPath, "entry_count": len(entries), "prompt": prompt})
			}
			fmt.Fprint(a.out, prompt)
			return nil
		},
	}
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "include open entries from every project")
	cmd.Flags().IntVar(&limit, "limit", 100, "maximum number of entries")
	return cmd
}

const agentInstructions = `When you encounter avoidable friction in tools, documentation, repository
structure, or the development environment, record it with:

    papercuts add "<concise observation>"

Log systemic friction that a maintainer could improve. Do not log ordinary
implementation work, expected test failures, or vague dissatisfaction. Continue
the current task after recording the observation.
`

func (a *app) instructionsCommand() *cobra.Command {
	return &cobra.Command{
		Use: "agent-instructions", Short: "Print instructions for AGENTS.md", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if a.jsonOutput {
				return writeJSON(a.out, map[string]string{"instructions": agentInstructions})
			}
			fmt.Fprint(a.out, agentInstructions)
			return nil
		},
	}
}

func (a *app) doctorCommand() *cobra.Command {
	return &cobra.Command{
		Use: "doctor", Short: "Show project and database diagnostics", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, err := a.context()
			if err != nil {
				return err
			}
			path := a.dbPath
			if path == "" {
				path, err = store.DefaultPath()
				if err != nil {
					return err
				}
			}
			result := map[string]any{"version": a.version, "repository_id": ctx.RepositoryID, "project_path": ctx.ProjectPath, "worktree_path": ctx.WorktreePath, "working_dir": ctx.WorkingDir, "database_path": path}
			if a.jsonOutput {
				return writeJSON(a.out, result)
			}
			fmt.Fprintf(a.out, "Version:    %s\nRepository: %s\nProject:    %s\nWorktree:   %s\nDirectory:  %s\nDatabase:   %s\n", a.version, ctx.RepositoryID, ctx.ProjectPath, ctx.WorktreePath, ctx.WorkingDir, path)
			return nil
		},
	}
}

func parseID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		return 0, &appError{code: 2, err: fmt.Errorf("invalid papercut ID %q", value)}
	}
	return id, nil
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func writeEntries(w io.Writer, entries []store.Entry, showProject bool) {
	if len(entries) == 0 {
		fmt.Fprintln(w, "No papercuts found.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if showProject {
		fmt.Fprintln(tw, "ID\tSTATUS\tPROJECT\tAGE\tOBSERVATION")
	} else {
		fmt.Fprintln(tw, "ID\tSTATUS\tAGE\tOBSERVATION")
	}
	for _, entry := range entries {
		status := "open"
		if entry.ResolvedAt != nil {
			status = "resolved"
		}
		created, _ := time.Parse(time.RFC3339Nano, entry.CreatedAt)
		age := humanDuration(time.Since(created))
		if showProject {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", entry.ID, status, filepath.Base(entry.ProjectPath), age, truncate(oneLine(entry.Body), 80))
		} else {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", entry.ID, status, age, truncate(oneLine(entry.Body), 80))
		}
	}
	tw.Flush()
}

func isTerminalReader(r io.Reader) bool {
	file, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func truncate(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max-1]) + "…"
}

func humanDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	switch {
	case duration < time.Minute:
		return "now"
	case duration < time.Hour:
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%dh", int(duration.Hours()))
	default:
		return fmt.Sprintf("%dd", int(duration.Hours()/24))
	}
}

func friendlyTime(value string) string {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return parsed.Local().Format("2006-01-02 15:04")
}

func pastTense(action string) string {
	if action == "resolve" {
		return "Resolved"
	}
	return "Reopened"
}

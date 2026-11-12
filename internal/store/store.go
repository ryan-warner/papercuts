package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("papercut not found")

type Entry struct {
	ID           int64   `json:"id"`
	RepositoryID string  `json:"repository_id"`
	ProjectPath  string  `json:"project_path"`
	WorktreePath string  `json:"worktree_path"`
	WorkingDir   string  `json:"working_dir"`
	Body         string  `json:"body"`
	CreatedAt    string  `json:"created_at"`
	ResolvedAt   *string `json:"resolved_at"`
}

type ListOptions struct {
	RepositoryID string
	AllProjects  bool
	State        string
	Limit        int
}

type Stats struct {
	RepositoryID     string  `json:"repository_id,omitempty"`
	ProjectPath      string  `json:"project_path,omitempty"`
	Open             int     `json:"open"`
	Resolved         int     `json:"resolved"`
	Total            int     `json:"total"`
	OldestOpenAt     *string `json:"oldest_open_at"`
	AverageResolveHr float64 `json:"average_resolution_hours"`
}

type ProjectSummary struct {
	RepositoryID string  `json:"repository_id"`
	ProjectPath  string  `json:"project_path"`
	Open         int     `json:"open"`
	Resolved     int     `json:"resolved"`
	LastSeenAt   *string `json:"last_seen_at"`
}

type Store struct {
	db   *sql.DB
	path string
}

func DefaultPath() (string, error) {
	if home := os.Getenv("PAPERCUTS_HOME"); home != "" {
		return filepath.Join(home, "papercuts.db"), nil
	}
	var base string
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, "Library", "Application Support")
	case "windows":
		base = os.Getenv("LOCALAPPDATA")
		if base == "" {
			return "", errors.New("LOCALAPPDATA is not set")
		}
	default:
		base = os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, ".local", "share")
		}
	}
	return filepath.Join(base, "papercuts", "papercuts.db"), nil
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, statement := range []string{
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialize database: %w", err)
		}
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	if err := enableWAL(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, path: path}, nil
}

func enableWAL(db *sql.DB) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := db.Exec("PRAGMA journal_mode = WAL")
		if err == nil {
			return nil
		}
		message := strings.ToLower(err.Error())
		if (!strings.Contains(message, "locked") && !strings.Contains(message, "busy")) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func migrate(db *sql.DB) error {
	ctx := context.Background()
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == 2 {
		return nil
	}
	if version > 2 {
		return fmt.Errorf("database version %d is newer than supported version 2", version)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(ctx, "ROLLBACK")
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == 2 {
		_, err := conn.ExecContext(ctx, "COMMIT")
		return err
	}
	if version > 2 {
		return fmt.Errorf("database version %d is newer than supported version 2", version)
	}
	if version == 0 {
		if _, err := conn.ExecContext(ctx, `CREATE TABLE entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			repository_id TEXT NOT NULL,
			project_path TEXT NOT NULL,
			worktree_path TEXT NOT NULL,
			working_dir TEXT NOT NULL,
			body TEXT NOT NULL CHECK(length(trim(body)) > 0),
			created_at TEXT NOT NULL,
			resolved_at TEXT
		)`); err != nil {
			return err
		}
	} else if version == 1 {
		for _, statement := range []string{
			"ALTER TABLE entries ADD COLUMN repository_id TEXT",
			"ALTER TABLE entries ADD COLUMN worktree_path TEXT",
			"UPDATE entries SET repository_id = project_path, worktree_path = project_path",
		} {
			if _, err := conn.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}
	if _, err := conn.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS entries_repository_open
		ON entries(repository_id, resolved_at, created_at)`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA user_version = 2"); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Path() string { return s.path }

func (s *Store) Add(ctx context.Context, repositoryID, projectPath, worktreePath, workingDir, body string) (Entry, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx,
		"INSERT INTO entries(repository_id, project_path, worktree_path, working_dir, body, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		repositoryID, projectPath, worktreePath, workingDir, body, now)
	if err != nil {
		return Entry{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Entry{}, err
	}
	return s.Get(ctx, id)
}

func (s *Store) Get(ctx context.Context, id int64) (Entry, error) {
	var entry Entry
	err := s.db.QueryRowContext(ctx, `SELECT id, repository_id, project_path, worktree_path, working_dir, body, created_at, resolved_at
		FROM entries WHERE id = ?`, id).Scan(&entry.ID, &entry.RepositoryID, &entry.ProjectPath, &entry.WorktreePath, &entry.WorkingDir, &entry.Body, &entry.CreatedAt, &entry.ResolvedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, ErrNotFound
	}
	return entry, err
}

func (s *Store) List(ctx context.Context, options ListOptions) ([]Entry, error) {
	query := `SELECT id, repository_id, project_path, worktree_path, working_dir, body, created_at, resolved_at FROM entries WHERE 1=1`
	args := make([]any, 0, 2)
	if !options.AllProjects {
		query += " AND repository_id = ?"
		args = append(args, options.RepositoryID)
	}
	switch options.State {
	case "open":
		query += " AND resolved_at IS NULL"
	case "resolved":
		query += " AND resolved_at IS NOT NULL"
	}
	query += " ORDER BY created_at DESC"
	if options.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, options.Limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		var entry Entry
		if err := rows.Scan(&entry.ID, &entry.RepositoryID, &entry.ProjectPath, &entry.WorktreePath, &entry.WorkingDir, &entry.Body, &entry.CreatedAt, &entry.ResolvedAt); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Store) SetResolved(ctx context.Context, ids []int64, resolved bool) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var affected int64
	for _, id := range ids {
		var result sql.Result
		if resolved {
			result, err = tx.ExecContext(ctx, "UPDATE entries SET resolved_at = ? WHERE id = ? AND resolved_at IS NULL", time.Now().UTC().Format(time.RFC3339Nano), id)
		} else {
			result, err = tx.ExecContext(ctx, "UPDATE entries SET resolved_at = NULL WHERE id = ? AND resolved_at IS NOT NULL", id)
		}
		if err != nil {
			return 0, err
		}
		n, _ := result.RowsAffected()
		affected += n
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return affected, nil
}

func (s *Store) Stats(ctx context.Context, repositoryID, projectPath string, allProjects bool) (Stats, error) {
	stats := Stats{RepositoryID: repositoryID, ProjectPath: projectPath}
	where := ""
	args := []any{}
	if !allProjects {
		where = " WHERE repository_id = ?"
		args = append(args, repositoryID)
	} else {
		stats.RepositoryID = ""
		stats.ProjectPath = ""
	}
	query := `SELECT
		COALESCE(SUM(CASE WHEN resolved_at IS NULL THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN resolved_at IS NOT NULL THEN 1 ELSE 0 END), 0),
		COUNT(*),
		MIN(CASE WHEN resolved_at IS NULL THEN created_at END),
		COALESCE(AVG(CASE WHEN resolved_at IS NOT NULL THEN (julianday(resolved_at)-julianday(created_at))*24 END), 0)
		FROM entries` + where
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&stats.Open, &stats.Resolved, &stats.Total, &stats.OldestOpenAt, &stats.AverageResolveHr)
	return stats, err
}

func (s *Store) Projects(ctx context.Context) ([]ProjectSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT repository_id, MAX(project_path),
		SUM(CASE WHEN resolved_at IS NULL THEN 1 ELSE 0 END),
		SUM(CASE WHEN resolved_at IS NOT NULL THEN 1 ELSE 0 END), MAX(created_at)
		FROM entries GROUP BY repository_id ORDER BY MAX(created_at) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []ProjectSummary
	for rows.Next() {
		var project ProjectSummary
		if err := rows.Scan(&project.RepositoryID, &project.ProjectPath, &project.Open, &project.Resolved, &project.LastSeenAt); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func (s *Store) AssociateRepository(ctx context.Context, repositoryID, projectPath string, legacyPaths []string) error {
	if len(legacyPaths) == 0 {
		legacyPaths = []string{projectPath}
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(legacyPaths)), ",")
	args := []any{repositoryID, projectPath, repositoryID}
	for _, path := range legacyPaths {
		args = append(args, path)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE entries SET repository_id = ?, project_path = ?
		WHERE repository_id = ? OR project_path IN (`+placeholders+")", args...)
	return err
}

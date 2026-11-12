package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
)

func TestEntryLifecycle(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "papercuts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	entry, err := s.Add(ctx, "/repo/.git", "/repo", "/repo", "/repo/pkg", "the fixture generator is undocumented")
	if err != nil {
		t.Fatal(err)
	}
	if entry.ID == 0 || entry.ResolvedAt != nil {
		t.Fatalf("unexpected entry: %#v", entry)
	}
	if n, err := s.SetResolved(ctx, []int64{entry.ID}, true); err != nil || n != 1 {
		t.Fatalf("resolve: n=%d err=%v", n, err)
	}
	stats, err := s.Stats(ctx, "/repo/.git", "/repo", false)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Open != 0 || stats.Resolved != 1 || stats.Total != 1 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}

func TestOpenMigratesVersionOneDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "papercuts.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_path TEXT NOT NULL,
			working_dir TEXT NOT NULL,
			body TEXT NOT NULL,
			created_at TEXT NOT NULL,
			resolved_at TEXT
		)`,
		`INSERT INTO entries(project_path, working_dir, body, created_at)
			VALUES ('/legacy', '/legacy/pkg', 'old entry', '2026-01-01T00:00:00Z')`,
		"PRAGMA user_version = 1",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	entry, err := s.Get(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if entry.RepositoryID != "/legacy" || entry.ProjectPath != "/legacy" || entry.WorktreePath != "/legacy" {
		t.Fatalf("unexpected migrated entry: %#v", entry)
	}
}

func TestConcurrentWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "papercuts.db")
	const writers = 8
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := Open(path)
			if err != nil {
				errs <- err
				return
			}
			defer s.Close()
			_, err = s.Add(context.Background(), "/repo/.git", "/repo", "/repo", "/repo", "parallel observation")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	stats, err := s.Stats(context.Background(), "/repo/.git", "/repo", false)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != writers {
		t.Fatalf("got %d entries, want %d", stats.Total, writers)
	}
}

func TestAssociateRepositoryMigratesLegacyWorktree(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "papercuts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	entry, err := s.Add(ctx, "/linked", "/linked", "/linked", "/linked/pkg", "legacy entry")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AssociateRepository(ctx, "/main/.git", "/main", []string{"/main", "/linked"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RepositoryID != "/main/.git" || got.ProjectPath != "/main" || got.WorktreePath != "/linked" {
		t.Fatalf("unexpected associated entry: %#v", got)
	}
}

func TestMergeDeletedLegacyProject(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "papercuts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if _, err := s.Add(ctx, "/deleted", "/deleted", "/deleted", "/deleted", "legacy entry"); err != nil {
		t.Fatal(err)
	}
	n, err := s.MergeProject(ctx, "/deleted", "/main/.git", "/main")
	if err != nil || n != 1 {
		t.Fatalf("merge: n=%d err=%v", n, err)
	}
	stats, err := s.Stats(ctx, "/main/.git", "/main", false)
	if err != nil || stats.Total != 1 {
		t.Fatalf("stats: %#v err=%v", stats, err)
	}
}

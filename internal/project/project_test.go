package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveGitRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "packages", "app")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(nested, "")
	if err != nil {
		t.Fatal(err)
	}
	wantRoot, _ := filepath.EvalSymlinks(root)
	wantNested, _ := filepath.EvalSymlinks(nested)
	if got.ProjectPath != wantRoot || got.WorkingDir != wantNested {
		t.Fatalf("unexpected context: %#v", got)
	}
}

func TestResolveLinkedWorktreeToPrimaryCheckout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	parent := t.TempDir()
	primary := filepath.Join(parent, "primary")
	linked := filepath.Join(parent, "linked")
	if err := os.Mkdir(primary, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, primary, "init")
	runGit(t, primary, "-c", "user.name=Papercuts Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial")
	runGit(t, primary, "worktree", "add", "-b", "linked-test", linked)
	nested := filepath.Join(linked, "pkg")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(nested, "")
	if err != nil {
		t.Fatal(err)
	}
	common, _ := filepath.EvalSymlinks(filepath.Join(primary, ".git"))
	wantPrimary, _ := filepath.EvalSymlinks(primary)
	wantLinked, _ := filepath.EvalSymlinks(linked)
	wantNested, _ := filepath.EvalSymlinks(nested)
	if got.RepositoryID != common || got.ProjectPath != wantPrimary || got.WorktreePath != wantLinked || got.WorkingDir != wantNested {
		t.Fatalf("unexpected context: %#v", got)
	}
	if len(got.LegacyProjectPaths) != 2 {
		t.Fatalf("unexpected legacy paths: %#v", got.LegacyProjectPaths)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func TestResolveFallsBackToWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	got, err := Resolve(root, "")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(root)
	if got.ProjectPath != want {
		t.Fatalf("got %q, want %q", got.ProjectPath, want)
	}
}

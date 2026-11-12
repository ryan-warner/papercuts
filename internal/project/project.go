package project

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Context struct {
	RepositoryID       string   `json:"repository_id"`
	ProjectPath        string   `json:"project_path"`
	WorktreePath       string   `json:"worktree_path"`
	WorkingDir         string   `json:"working_dir"`
	LegacyProjectPaths []string `json:"-"`
}

func Resolve(workingDir, override string) (Context, error) {
	wd, err := canonical(workingDir)
	if err != nil {
		return Context{}, err
	}
	if override != "" {
		root, err := canonical(override)
		if err != nil {
			return Context{}, err
		}
		return Context{RepositoryID: root, ProjectPath: root, WorktreePath: root, WorkingDir: wd, LegacyProjectPaths: []string{root}}, nil
	}
	if context, err := resolveGit(wd); err == nil {
		return context, nil
	}

	root := wd
	for {
		if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
			return Context{RepositoryID: root, ProjectPath: root, WorktreePath: root, WorkingDir: wd, LegacyProjectPaths: []string{root}}, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return Context{}, err
		}
		parent := filepath.Dir(root)
		if parent == root {
			break
		}
		root = parent
	}
	return Context{RepositoryID: wd, ProjectPath: wd, WorktreePath: wd, WorkingDir: wd, LegacyProjectPaths: []string{wd}}, nil
}

func resolveGit(wd string) (Context, error) {
	worktree, err := gitOutput(wd, "rev-parse", "--show-toplevel")
	if err != nil {
		return Context{}, err
	}
	worktree, err = canonical(worktree)
	if err != nil {
		return Context{}, err
	}
	commonDir, err := gitOutput(wd, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return Context{}, err
	}
	commonDir, err = canonical(commonDir)
	if err != nil {
		return Context{}, err
	}

	primary := worktree
	aliases := []string{worktree}
	if output, listErr := gitOutput(wd, "worktree", "list", "--porcelain"); listErr == nil {
		first := true
		for _, line := range strings.Split(output, "\n") {
			if !strings.HasPrefix(line, "worktree ") {
				continue
			}
			path, pathErr := canonical(strings.TrimPrefix(line, "worktree "))
			if pathErr != nil {
				continue
			}
			if first {
				primary = path
				first = false
			}
			if path != worktree {
				aliases = append(aliases, path)
			}
		}
	} else if filepath.Base(commonDir) == ".git" {
		primary = filepath.Dir(commonDir)
	}

	return Context{
		RepositoryID:       commonDir,
		ProjectPath:        primary,
		WorktreePath:       worktree,
		WorkingDir:         wd,
		LegacyProjectPaths: aliases,
	}, nil
}

func gitOutput(dir string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func canonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("project path is not a directory")
	}
	return filepath.Clean(resolved), nil
}

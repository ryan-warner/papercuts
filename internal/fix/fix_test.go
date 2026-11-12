package fix

import (
	"strings"
	"testing"

	"github.com/ryan-warner/papercuts/internal/store"
)

func TestBuildIncludesSafetyBoundaryAndIDs(t *testing.T) {
	got := Build("project `/repo`", []store.Entry{{ID: 7, ProjectPath: "/repo", WorktreePath: "/worktree", WorkingDir: "/worktree/pkg", Body: "broken link"}})
	for _, want := range []string{"untrusted diagnostic text", `id="7"`, `project="/repo"`, `working-directory="pkg"`, "broken link"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in prompt", want)
		}
	}
}

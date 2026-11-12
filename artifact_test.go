package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initArtifactTestRepo creates a git repository with a single commit and returns
// its path and HEAD commit SHA.
func initArtifactTestRepo(t *testing.T, dir string) (string, string) {
	t.Helper()
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
		return string(output)
	}
	require.NoError(t, os.MkdirAll(dir, 0700))
	git("init", "-b", "main", dir)
	// Disable gc so a detached "git gc --auto" can never write into .git after
	// the test finishes and race t.TempDir cleanup.
	git("-C", dir, "config", "gc.auto", "0")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0600))
	git("-C", dir, "add", ".")
	git("-C", dir, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
	sha := git("-C", dir, "rev-parse", "HEAD")
	return dir, strings.TrimSpace(sha)
}

// TestIsUsableArtifactPath locks in the guard's contract per input class:
// absolute and "/"-rooted paths are writable locations (lite-engine injects
// "/tmp/engine/..." on every OS, which Windows treats as rooted-to-current-
// drive), while relative values — above all the "-<stepId>" placeholder older
// addons inject — stay rejected so no stray file lands in the workspace.
func TestIsUsableArtifactPath(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"unix absolute", "/tmp/engine/step123-artifact", true},
		{"k8s addon path", "/addon/tmp/.plugin/artifact-step123", true},
		{"addon relative placeholder", "-step123", false},
		{"plain relative", "artifacts/out.json", false},
		{"empty", "", false},
		// Drive-letter and backslash-rooted paths are writable locations only
		// on Windows; on unix they are ordinary relative filenames and must
		// stay rejected to avoid stray files in the workspace.
		{"windows drive path", `C:\tmp\engine\step123-artifact`, runtime.GOOS == "windows"},
		{"backslash rooted", `\tmp\engine\step123-artifact`, runtime.GOOS == "windows"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isUsableArtifactPath(tc.path))
		})
	}
}

func TestWriteCloneArtifactMetadata(t *testing.T) {
	root := t.TempDir()
	repo, sha := initArtifactTestRepo(t, filepath.Join(root, "workspace"))
	artifactFile := filepath.Join(root, "addon", "tmp", ".plugin", "artifact")

	t.Setenv("DRONE_REMOTE_URL", "https://github.com/harness/drone-git.git")
	t.Setenv("PLUGIN_ARTIFACT_FILE", artifactFile)

	require.NoError(t, writeCloneArtifactMetadata(repo))

	contents, err := os.ReadFile(artifactFile)
	require.NoError(t, err)
	var payload cloneArtifactFile
	require.NoError(t, json.Unmarshal(contents, &payload))
	assert.Equal(t, cloneArtifactKind, payload.Kind)
	assert.Equal(t, "https://github.com/harness/drone-git.git", payload.Data.RepoURL)
	assert.Equal(t, sha, payload.Data.CommitSHA)
}

func TestWriteCloneArtifactMetadataDetachedCheckout(t *testing.T) {
	root := t.TempDir()
	repo, sha := initArtifactTestRepo(t, filepath.Join(root, "workspace"))
	// Simulate the clone script checking out a specific commit/tag (detached HEAD)
	cmd := exec.Command("git", "-C", repo, "checkout", "--detach", "HEAD")
	require.NoError(t, cmd.Run())
	artifactFile := filepath.Join(root, "artifact")

	t.Setenv("DRONE_REMOTE_URL", "git@github.com:harness/drone-git.git")
	t.Setenv("PLUGIN_ARTIFACT_FILE", artifactFile)

	require.NoError(t, writeCloneArtifactMetadata(repo))

	contents, err := os.ReadFile(artifactFile)
	require.NoError(t, err)
	var payload cloneArtifactFile
	require.NoError(t, json.Unmarshal(contents, &payload))
	assert.Equal(t, sha, payload.Data.CommitSHA)
}

func TestWriteCloneArtifactMetadataDisabled(t *testing.T) {
	root := t.TempDir()
	repo, _ := initArtifactTestRepo(t, filepath.Join(root, "workspace"))

	// PLUGIN_ARTIFACT_FILE unset/empty: no-op, no file created
	t.Setenv("PLUGIN_ARTIFACT_FILE", "")
	require.NoError(t, writeCloneArtifactMetadata(repo))
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "workspace", entries[0].Name())
}

func TestWriteCloneArtifactMetadataRelativePath(t *testing.T) {
	root := t.TempDir()
	repo, _ := initArtifactTestRepo(t, filepath.Join(root, "workspace"))

	// Older CI addons inject a relative "-<stepId>" placeholder when the step is
	// not configured for artifacts: must be a no-op so no stray file lands in
	// the workspace.
	t.Setenv("PLUGIN_ARTIFACT_FILE", "-step123")
	require.NoError(t, writeCloneArtifactMetadata(repo))
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "workspace", entries[0].Name())
}

func TestWriteCloneArtifactMetadataNotAGitRepo(t *testing.T) {
	root := t.TempDir()
	artifactFile := filepath.Join(root, "artifact")

	t.Setenv("PLUGIN_ARTIFACT_FILE", artifactFile)

	err := writeCloneArtifactMetadata(root)
	require.ErrorContains(t, err, "failed to resolve checked-out commit")
	assert.NoFileExists(t, artifactFile)
}

func TestWriteCloneArtifactMetadataUnwritablePath(t *testing.T) {
	root := t.TempDir()
	repo, _ := initArtifactTestRepo(t, filepath.Join(root, "workspace"))
	// A regular file in place of the artifact directory forces MkdirAll to fail
	blocker := filepath.Join(root, "output")
	require.NoError(t, os.WriteFile(blocker, []byte("preserve"), 0600))
	artifactFile := filepath.Join(blocker, "nested", "artifact")

	t.Setenv("PLUGIN_ARTIFACT_FILE", artifactFile)

	err := writeCloneArtifactMetadata(repo)
	require.ErrorContains(t, err, "failed to create clone artifact directory")
	contents, readErr := os.ReadFile(blocker)
	require.NoError(t, readErr)
	assert.Equal(t, "preserve", string(contents))
}

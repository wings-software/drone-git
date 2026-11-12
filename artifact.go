package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/exp/slog"
)

const cloneArtifactKind = "gitClone/v1"

// cloneArtifactFile is the envelope written to PLUGIN_ARTIFACT_FILE.
type cloneArtifactFile struct {
	Kind string            `json:"kind"`
	Data cloneArtifactData `json:"data"`
}

// cloneArtifactData carries the git-clone metadata consumed by CI manager.
type cloneArtifactData struct {
	RepoURL   string `json:"repoUrl,omitempty"`
	CommitSHA string `json:"commitSha,omitempty"`
}

// writeCloneArtifactMetadata publishes the cloned repository URL and the commit
// actually checked out, enabling artifact-to-commit traceability when pipelines
// clone via explicit Git Clone steps instead of the pipeline codebase (CI-25945).
// It is a no-op when PLUGIN_ARTIFACT_FILE is unset (e.g. local runs) or not a
// usable absolute/rooted path. The path check matters because older CI addons
// always inject PLUGIN_ARTIFACT_FILE, defaulting it to a relative "-<stepId>"
// value when the step was not configured for artifacts; writing there would
// drop a stray file into the workspace that shows up in the user's git status.
func writeCloneArtifactMetadata(workdir string) error {
	artifactFile := os.Getenv("PLUGIN_ARTIFACT_FILE")
	if artifactFile == "" || !isUsableArtifactPath(artifactFile) {
		slog.Debug("PLUGIN_ARTIFACT_FILE unset or not a usable path, skipping clone artifact metadata", "file", artifactFile)
		return nil
	}

	sha, err := resolveCheckedOutCommit(workdir)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(cloneArtifactFile{
		Kind: cloneArtifactKind,
		Data: cloneArtifactData{
			RepoURL:   getRepositoryURL(),
			CommitSHA: sha,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal clone artifact metadata: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(artifactFile), 0700); err != nil {
		return fmt.Errorf("failed to create clone artifact directory: %v", err)
	}
	if err := os.WriteFile(artifactFile, payload, 0600); err != nil {
		return fmt.Errorf("failed to write clone artifact file %s: %v", artifactFile, err)
	}
	slog.Debug("Wrote clone artifact metadata", "file", artifactFile)
	return nil
}

// isUsableArtifactPath reports whether p is a location the OS can genuinely
// write to, as opposed to the relative "-<stepId>" placeholder older CI addons
// inject for steps that are not configured for artifacts. Besides absolute
// paths it accepts "/"-rooted paths: Windows resolves them against the current
// drive even though filepath.IsAbs rejects them there, and lite-engine hands
// Windows VMs a hardcoded "/tmp/engine/..." value. On unix this accepts exactly
// the same paths as filepath.IsAbs (IsAbs there is "starts with /"), so
// behavior on unix is unchanged. A leading backslash is rooted only on Windows;
// on unix it would be a plain relative filename, so it stays rejected.
func isUsableArtifactPath(p string) bool {
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return true
	}
	return runtime.GOOS == "windows" && strings.HasPrefix(p, `\`)
}

// resolveCheckedOutCommit returns the commit currently checked out in workdir.
func resolveCheckedOutCommit(workdir string) (string, error) {
	output, err := exec.Command("git", "-C", workdir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("failed to resolve checked-out commit: %v", err)
	}
	sha := strings.TrimSpace(string(output))
	if sha == "" {
		return "", fmt.Errorf("failed to resolve checked-out commit: git rev-parse HEAD returned empty output")
	}
	return sha, nil
}

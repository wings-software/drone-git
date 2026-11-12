package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slog"
)

func TestMainCloneMetadata(t *testing.T) {
	if os.Getenv("DRONE_GIT_MAIN_TEST_HELPER") == "1" {
		main()
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	args := []string{"-test.run=^TestMainCloneMetadata$"}
	// Share Go's coverage directory so the child process contributes to the report.
	if coverage := flag.Lookup("test.gocoverdir"); coverage != nil && coverage.Value.String() != "" {
		args = append(args, "-test.gocoverdir="+coverage.Value.String())
	}
	for _, test := range []struct {
		name          string
		outputFailure bool
		noBash        bool
	}{
		{name: "successful metadata"},
		{name: "metadata failure preserves successful clone", outputFailure: true},
		{name: "minimal image without bash", noBash: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.noBash && runtime.GOOS == "windows" {
				t.Skip("Windows uses PowerShell")
			}
			root := t.TempDir()
			remote := filepath.Join(root, "remote")
			workspace := filepath.Join(root, "workspace")
			config := filepath.Join(root, "git-config")
			scripts := filepath.Join(root, "scripts")
			require.NoError(t, os.MkdirAll(config, 0700))
			require.NoError(t, os.MkdirAll(scripts, 0700))
			for key, value := range map[string]string{
				"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_COUNT": "0",
				"GIT_CONFIG_GLOBAL":         filepath.Join(config, ".gitconfig"),
				"HARNESS_GIT_CONFIG_FOLDER": config, "HARNESS_WORKDIR": scripts,
				"DRONE_REMOTE_URL": remote, "DRONE_WORKSPACE": workspace,
				"DRONE_BUILD_EVENT": "push", "DRONE_COMMIT_BRANCH": "main",
				"DRONE_COMMIT_REF": "", "DRONE_NETRC_MACHINE": "", "DRONE_SSH_KEY": "",
				"DRONE_NETRC_LFS_ENABLED": "false", "DRONE_NETRC_FETCH_TAGS": "false",
				"DRONE_NETRC_SPARSE_CHECKOUT": "", "DRONE_NETRC_PRE_FETCH": "",
				"DRONE_NETRC_SUBMODULE_STRATEGY": "", "DRONE_PERSIST_CREDS": "",
				"HARNESS_GIT_PROXY": "false", "PLUGIN_ONLY_COPY_FILE_CONTENT": "false",
				"CI_DISABLE_TELEMETRY": "", "DISABLE_SCC_METRICS": "",
			} {
				t.Setenv(key, value)
			}
			git := func(args ...string) string {
				cmd := exec.Command("git", args...)
				output, err := cmd.CombinedOutput()
				require.NoError(t, err, "%s", output)
				return strings.TrimSpace(string(output))
			}
			git("init", "-b", "main", remote)
			for file, contents := range map[string]string{
				"main.go": "package main\nfunc main() {}\n",
				"app.js":  "console.log('test');\n", "package.json": "{\"name\":\"fixture\"}\n",
			} {
				require.NoError(t, os.WriteFile(filepath.Join(remote, file), []byte(contents), 0600))
			}
			git("-C", remote, "add", ".")
			git("-C", remote, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
			sha := git("-C", remote, "rev-parse", "HEAD")
			if test.noBash {
				t.Setenv("GIT_EXEC_PATH", git("--exec-path"))
				bin := filepath.Join(root, "bin")
				require.NoError(t, os.MkdirAll(bin, 0700))
				for _, tool := range []string{"sh", "git", "dirname", "mkdir", "chmod", "touch", "cat", "find", "sed", "sort", "uniq", "tr", "cut", "grep"} {
					path, err := exec.LookPath(tool)
					require.NoError(t, err)
					require.NoError(t, os.Symlink(path, filepath.Join(bin, tool)))
				}
				t.Setenv("PATH", bin)
				_, err := exec.LookPath("bash")
				require.ErrorIs(t, err, exec.ErrNotFound)
			}
			t.Setenv("DRONE_COMMIT_SHA", sha)
			output := filepath.Join(root, "output", "nested", "metadata.json")
			if test.outputFailure {
				require.NoError(t, os.WriteFile(filepath.Join(root, "output"), []byte("preserve"), 0600))
			} else {
				require.NoError(t, os.MkdirAll(filepath.Dir(output), 0700))
			}
			t.Setenv("PLUGIN_BUILD_TOOL_FILE", output)
			cmd := exec.Command(executable, args...)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "DRONE_GIT_MAIN_TEST_HELPER=1")
			logOutput, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", logOutput)

			assert.Equal(t, sha, git("-C", workspace, "rev-parse", "HEAD"))
			assert.FileExists(t, filepath.Join(workspace, "main.go"))
			entries, err := os.ReadDir(scripts)
			require.NoError(t, err)
			assert.Empty(t, entries, "temporary embedded scripts should be removed")
			if test.outputFailure {
				contents, err := os.ReadFile(filepath.Join(root, "output"))
				require.NoError(t, err)
				assert.Equal(t, "preserve", string(contents))
				return
			}
			contents, err := os.ReadFile(output)
			require.NoError(t, err)
			var data BuildToolData
			require.NoError(t, json.Unmarshal(contents, &data))
			assert.Equal(t, remote, data.Repository)
			assert.Equal(t, "branch", data.BuildEvent)
			assert.Equal(t, "main", data.BuildEventValue)
			assert.Equal(t, "Node", data.HarnessBuildTool)
			assert.Contains(t, data.HarnessLang, "Golang")
			assert.Contains(t, data.HarnessLang, "JavaScript")
			assert.Greater(t, data.Metrics.Files, int64(0))
			assert.Equal(t, getPluginVersion(), data.PluginVersion)
		})
	}
}

// TestMainCloneArtifactMetadata runs the real binary end-to-end against a local
// git remote and verifies the gitClone/v1 artifact receipt (repo URL +
// checked-out commit) is written to PLUGIN_ARTIFACT_FILE after the clone
// (CI-25945). Kept separate from TestMainCloneMetadata so artifact assertions
// can never fail the pre-existing metadata scenarios.
func TestMainCloneArtifactMetadata(t *testing.T) {
	if os.Getenv("DRONE_GIT_MAIN_ARTIFACT_TEST_HELPER") == "1" {
		main()
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	args := []string{"-test.run=^TestMainCloneArtifactMetadata$"}
	// Share Go's coverage directory so the child process contributes to the report.
	if coverage := flag.Lookup("test.gocoverdir"); coverage != nil && coverage.Value.String() != "" {
		args = append(args, "-test.gocoverdir="+coverage.Value.String())
	}

	root := t.TempDir()
	remote := filepath.Join(root, "remote")
	workspace := filepath.Join(root, "workspace")
	config := filepath.Join(root, "git-config")
	scripts := filepath.Join(root, "scripts")
	require.NoError(t, os.MkdirAll(config, 0700))
	require.NoError(t, os.MkdirAll(scripts, 0700))
	for key, value := range map[string]string{
		"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_COUNT": "0",
		"GIT_CONFIG_GLOBAL":         filepath.Join(config, ".gitconfig"),
		"HARNESS_GIT_CONFIG_FOLDER": config, "HARNESS_WORKDIR": scripts,
		"DRONE_REMOTE_URL": remote, "DRONE_WORKSPACE": workspace,
		"DRONE_BUILD_EVENT": "push", "DRONE_COMMIT_BRANCH": "main",
		"DRONE_COMMIT_REF": "", "DRONE_NETRC_MACHINE": "", "DRONE_SSH_KEY": "",
		"DRONE_NETRC_LFS_ENABLED": "false", "DRONE_NETRC_FETCH_TAGS": "false",
		"DRONE_NETRC_SPARSE_CHECKOUT": "", "DRONE_NETRC_PRE_FETCH": "",
		"DRONE_NETRC_SUBMODULE_STRATEGY": "", "DRONE_PERSIST_CREDS": "",
		"HARNESS_GIT_PROXY": "false", "PLUGIN_ONLY_COPY_FILE_CONTENT": "false",
		"CI_DISABLE_TELEMETRY": "", "DISABLE_SCC_METRICS": "",
	} {
		t.Setenv(key, value)
	}
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
		return strings.TrimSpace(string(output))
	}
	git("init", "-b", "main", remote)
	require.NoError(t, os.WriteFile(filepath.Join(remote, "main.go"), []byte("package main\n"), 0600))
	git("-C", remote, "add", ".")
	git("-C", remote, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
	sha := git("-C", remote, "rev-parse", "HEAD")
	t.Setenv("DRONE_COMMIT_SHA", sha)
	output := filepath.Join(root, "output", "metadata.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(output), 0700))
	t.Setenv("PLUGIN_BUILD_TOOL_FILE", output)
	artifact := filepath.Join(root, "addon", "tmp", ".plugin", "artifact")
	t.Setenv("PLUGIN_ARTIFACT_FILE", artifact)

	cmd := exec.Command(executable, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "DRONE_GIT_MAIN_ARTIFACT_TEST_HELPER=1")
	logOutput, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", logOutput)

	assert.Equal(t, sha, git("-C", workspace, "rev-parse", "HEAD"))
	assert.FileExists(t, filepath.Join(workspace, "main.go"))
	artifactContents, err := os.ReadFile(artifact)
	require.NoError(t, err)
	var payload cloneArtifactFile
	require.NoError(t, json.Unmarshal(artifactContents, &payload))
	assert.Equal(t, cloneArtifactKind, payload.Kind)
	assert.Equal(t, remote, payload.Data.RepoURL)
	assert.Equal(t, sha, payload.Data.CommitSHA)
}

func TestRunGitCloneInvalidTempDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(path, nil, 0600))
	t.Setenv("HARNESS_WORKDIR", path)
	t.Cleanup(cleanupTempDir)
	require.ErrorContains(t, runGitClone(), "failed to create temp dir")
	output := filepath.Join(t.TempDir(), "metadata.json")
	t.Setenv("PLUGIN_BUILD_TOOL_FILE", output)
	collectAndWriteMetrics(t.TempDir())
	assert.NoFileExists(t, output)
}

func TestMetadataContext(t *testing.T) {
	for _, test := range []struct {
		event, tag, branch, source, sha string
		wantEvent, wantValue            string
	}{
		{event: "tag", tag: "v1", wantEvent: "tag", wantValue: "v1"},
		{event: "tag", sha: "abc", wantEvent: "tag"},
		{event: "pull_request", source: "feature", wantEvent: "pull_request", wantValue: "feature"},
		{event: "pull_request", wantEvent: "pull_request"},
		{event: "push", branch: "main", wantEvent: "branch", wantValue: "main"},
		{event: "push", tag: "stale-tag", wantEvent: "push"},
		{tag: "v2", sha: "abc", wantEvent: "tag", wantValue: "v2"},
		{sha: "abc", wantEvent: "commit", wantValue: "abc"},
		{},
	} {
		t.Run(test.event+"/"+test.wantValue, func(t *testing.T) {
			for key, value := range map[string]string{
				"DRONE_BUILD_EVENT": test.event, "DRONE_TAG": test.tag,
				"DRONE_COMMIT_BRANCH": test.branch, "DRONE_SOURCE_BRANCH": test.source,
				"DRONE_COMMIT_SHA": test.sha,
			} {
				t.Setenv(key, value)
			}
			event, value := getBuildEventInfo()
			assert.Equal(t, test.wantEvent, event)
			assert.Equal(t, test.wantValue, value)
		})
	}
	t.Setenv("DRONE_WORKSPACE", "")
	workdir, err := os.Getwd()
	require.NoError(t, err)
	workspace, err := getWorkspaceDirectory()
	require.NoError(t, err)
	assert.Equal(t, workdir, workspace)
	t.Setenv("DRONE_REMOTE_URL", "")
	assert.Empty(t, getRepositoryURL())
	original := version
	t.Cleanup(func() { version = original })
	version = "v1.7.27"
	assert.Equal(t, version, getPluginVersion())
	version = "dev"
	assert.Equal(t, "1.0.0", getPluginVersion())
}

func TestFindPowerShell(t *testing.T) {
	for _, test := range []struct {
		name  string
		files []string
		want  string
	}{
		{"prefer Core", []string{"pwsh", "powershell"}, "pwsh"},
		{"legacy fallback", []string{"powershell"}, "powershell"},
		{"explicit exe fallback", []string{"powershell.exe"}, "powershell.exe"},
		{"missing executables", nil, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			filename := func(name string) string {
				if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
					name += ".exe"
				}
				return filepath.Join(dir, name)
			}
			for _, name := range test.files {
				require.NoError(t, os.WriteFile(filename(name), nil, 0755))
			}
			t.Setenv("PATH", dir)
			var logs bytes.Buffer
			originalLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(originalLogger) })
			want := "pwsh"
			if test.want != "" {
				want = filename(test.want)
			}
			assert.Equal(t, want, findPowerShell())
			if test.want == "" {
				var entry struct {
					Checked []string `json:"checked"`
				}
				require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
				assert.Equal(t, []string{"pwsh", "powershell", "powershell.exe"}, entry.Checked)
				assert.NotContains(t, logs.String(), "!BADKEY")
			}
		})
	}
}

func TestMetadataUnavailable(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "metadata.json")
	t.Setenv("PLUGIN_BUILD_TOOL_FILE", output)
	t.Setenv("CI_DISABLE_TELEMETRY", "")
	t.Setenv("DISABLE_SCC_METRICS", "true")
	original := globalTmpDir
	t.Cleanup(func() { globalTmpDir = original })
	globalTmpDir = ""

	// Analytics must not run before clone scripts have been initialized.
	require.ErrorContains(t, executeBuildToolScript(root), "temp directory not initialized")
	require.NoError(t, tryCollectAndWriteMetrics(root))
	assert.NoFileExists(t, output)

	// A missing provider script and malformed output still produce valid fallback JSON.
	globalTmpDir = t.TempDir()
	require.NoError(t, os.WriteFile(output, []byte("invalid JSON"), 0600))
	require.NoError(t, tryCollectAndWriteMetrics(root))
	contents, err := os.ReadFile(output)
	require.NoError(t, err)
	var data BuildToolData
	require.NoError(t, json.Unmarshal(contents, &data))
	assert.Empty(t, data.HarnessLang)
	assert.Empty(t, data.HarnessBuildTool)
	assert.Zero(t, data.Metrics.Files)

	// An output path that is a directory must return an error and preserve the directory.
	t.Setenv("PLUGIN_BUILD_TOOL_FILE", root)
	require.ErrorContains(t, writeBuildToolFile(&data), "failed to write build tool file")
	collectAndWriteMetrics(root)
	assert.DirExists(t, root)
	t.Setenv("PLUGIN_BUILD_TOOL_FILE", "")
	require.NoError(t, executeBuildToolScript(root))
	require.NoError(t, writeBuildToolFile(&data))
}

func TestWriteScriptsConflict(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "posix", "clone")
	require.NoError(t, os.MkdirAll(path, 0700))
	require.ErrorContains(t, writeScriptsToTemp(root), "failed to write file")
	assert.DirExists(t, path)
}

func TestWorkspaceDirectoryUnavailable(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux reports an error for an unlinked working directory")
	}
	path := filepath.Join(t.TempDir(), "workspace")
	require.NoError(t, os.Mkdir(path, 0700))
	t.Chdir(path)
	require.NoError(t, os.Remove(path))
	t.Setenv("DRONE_WORKSPACE", "")
	_, err := getWorkspaceDirectory()
	require.ErrorContains(t, err, "cannot get working directory")
}

func TestWriteScriptsToTemp(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "drone-git-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Test writing scripts
	err = writeScriptsToTemp(tmpDir)
	require.NoError(t, err)

	// Verify posix scripts are written
	posixFiles := []string{"script", "clone", "common"}
	for _, file := range posixFiles {
		path := filepath.Join(tmpDir, "posix", file)
		_, err := os.Stat(path)
		assert.NoError(t, err, "posix file should exist: %s", file)

		// Verify file permissions
		info, err := os.Stat(path)
		assert.NoError(t, err)
		assert.Equal(t, mode, info.Mode().Perm(), "file should have correct permissions: %s", file)
	}

	// Verify windows scripts are written
	windowsFiles := []string{"clone.ps1", "common.ps1"}
	for _, file := range windowsFiles {
		path := filepath.Join(tmpDir, "windows", file)
		_, err := os.Stat(path)
		assert.NoError(t, err, "windows file should exist: %s", file)

		// Verify file permissions
		info, err := os.Stat(path)
		assert.NoError(t, err)
		assert.Equal(t, mode, info.Mode().Perm(), "file should have correct permissions: %s", file)
	}
}

func TestRunCmds(t *testing.T) {
	tests := []struct {
		name        string
		cmds        []*exec.Cmd
		env         []string
		workdir     string
		wantErr     bool
		wantOutput  string
		wantErrText string
	}{
		{
			name: "successful command",
			cmds: []*exec.Cmd{
				exec.Command("echo", "hello"),
			},
			env:        []string{"TEST=true"},
			workdir:    ".",
			wantErr:    false,
			wantOutput: "hello\n",
		},
		{
			name: "failed command",
			cmds: []*exec.Cmd{
				exec.Command("nonexistent-command"),
			},
			env:     []string{"TEST=true"},
			workdir: ".",
			wantErr: true,
		},
		{
			name: "multiple commands",
			cmds: []*exec.Cmd{
				exec.Command("echo", "first"),
				exec.Command("echo", "second"),
			},
			env:        []string{"TEST=true"},
			workdir:    ".",
			wantErr:    false,
			wantOutput: "first\nsecond\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}

			err := runCmds(tt.cmds, tt.env, tt.workdir, stdout, stderr)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrText != "" {
					assert.Contains(t, stderr.String(), tt.wantErrText)
				}
			} else {
				assert.NoError(t, err)
				if tt.wantOutput != "" {
					assert.Equal(t, tt.wantOutput, stdout.String())
				}
			}
		})
	}
}

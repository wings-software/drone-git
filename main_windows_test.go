package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWindowsPreFetch(t *testing.T) {
	scripts := filepath.Join(t.TempDir(), "scripts with spaces")
	require.NoError(t, writeScriptsToTemp(scripts))
	for _, test := range []struct {
		name    string
		command string
		wantErr bool
	}{
		{
			name: "multiline block",
			command: "if (-not (Test-Path preFetchDir)) {\n" +
				"  New-Item -ItemType Directory preFetchDir | Out-Null\n" +
				"}\ngit status --short",
		},
		{
			name:    "native error followed by successful command",
			command: "git definitely-not-a-valid-subcommand\nNew-Item -ItemType File unexpected-prefetch-output | Out-Null\ngit status --short",
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			workdir := t.TempDir()
			t.Setenv("DRONE_REMOTE_URL", "https://example.invalid/not-fetched.git")
			t.Setenv("DRONE_NETRC_PRE_FETCH", test.command)
			for _, name := range []string{"HARNESS_GIT_CONFIG_FOLDER", "HARNESS_GIT_PROXY", "DRONE_NETRC_DEBUG", "DRONE_NETRC_FETCH_TAGS", "DRONE_NETRC_LFS_ENABLED", "DRONE_NETRC_SPARSE_CHECKOUT"} {
				t.Setenv(name, "")
			}
			cmd := exec.Command(findPowerShell(), "-NoProfile", "-NonInteractive", "-File", filepath.Join(scripts, "windows", "common.ps1"))
			cmd.Dir = workdir
			output, err := cmd.CombinedOutput()
			if test.wantErr {
				require.Error(t, err, "%s", output)
				require.NoFileExists(t, filepath.Join(workdir, "unexpected-prefetch-output"))
			} else {
				require.NoError(t, err, "%s", output)
				_, err = os.Stat(filepath.Join(workdir, "preFetchDir"))
				require.NoError(t, err)
			}
		})
	}
}

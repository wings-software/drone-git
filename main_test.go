package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

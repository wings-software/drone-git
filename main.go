package main

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/exp/slog"
)

const mode os.FileMode = 0755

//go:embed posix/* windows/*
var scriptFS embed.FS // embedding both posix and windows directory scripts to be available to the binary

func writeScriptsToTemp(tmpDir string) error {
	// Walk through the embedded filesystem and write all files
	return fs.WalkDir(scriptFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory
		if path == "." {
			return nil
		}

		dstPath := filepath.Join(tmpDir, path)

		if d.IsDir() {
			return os.MkdirAll(dstPath, mode)
		}

		// Read and write the file
		content, err := scriptFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %v", path, err)
		}

		if err := os.MkdirAll(filepath.Dir(dstPath), mode); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", filepath.Dir(dstPath), err)
		}

		if err := os.WriteFile(dstPath, content, mode); err != nil {
			return fmt.Errorf("failed to write file %s: %v", dstPath, err)
		}

		return nil
	})
}

func runGitClone() error {
	// Create a unique temporary subdirectory
	tmpDir, err := os.MkdirTemp("", "drone-git-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := writeScriptsToTemp(tmpDir); err != nil {
		return err
	}

	ctx := context.Background()

	// current working directory (workspace)
	workdir, err := os.Getwd()
	if err != nil {
		slog.Error("cannot get workdir", "error", err)
		os.Exit(1)
	}

	switch runtime.GOOS {
	case "windows":
		scriptPath := filepath.Join(tmpDir, "windows", "clone.ps1")
		script := fmt.Sprintf(
			"$ErrorActionPreference = 'Stop'; $ProgressPreference = 'SilentlyContinue'; %s",
			scriptPath)
		cmd := exec.Command("pwsh", "-Command", script)
		return runCmds(ctx, []*exec.Cmd{cmd}, os.Environ(), workdir, os.Stdout, os.Stderr)

	case "linux", "darwin":
		shell := "bash"
		if _, err := exec.LookPath("bash"); err != nil {
			shell = "sh"
		}

		scriptPath := filepath.Join(tmpDir, "posix", "script")
		cmd := exec.Command(shell, scriptPath)
		return runCmds(ctx, []*exec.Cmd{cmd}, os.Environ(), workdir, os.Stdout, os.Stderr)

	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func runCmds(ctx context.Context, cmds []*exec.Cmd, env []string, workdir string,
	stdout io.Writer, stderr io.Writer) error {
	for _, cmd := range cmds {
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		cmd.Env = env
		cmd.Dir = workdir
		trace(cmd)

		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

func trace(cmd *exec.Cmd) {
	s := fmt.Sprintf("+ %s\n", strings.Join(cmd.Args, " "))
	slog.Debug(s)
}

func main() {
	if err := runGitClone(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

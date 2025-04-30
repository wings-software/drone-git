package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

//go:embed posix/* windows/*
var scriptFS embed.FS

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
			return os.MkdirAll(dstPath, 0755)
		}

		// Read and write the file
		content, err := scriptFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %v", path, err)
		}

		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", filepath.Dir(dstPath), err)
		}

		var mode os.FileMode = 0755
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

	// List contents of temp directory
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		fmt.Printf("Error reading temp dir: %v\n", err)
	} else {
		fmt.Printf("Contents of %s:\n", tmpDir)
		for _, entry := range entries {
			fmt.Printf("%s\n", entry.Name())
		}
	}

	ctx := context.Background()

	switch runtime.GOOS {
	case "windows":
		scriptPath := filepath.Join(tmpDir, "windows", "clone.ps1")
		script := fmt.Sprintf(
			"$ErrorActionPreference = 'Stop'; $ProgressPreference = 'SilentlyContinue'; %s",
			scriptPath)
		cmd := exec.CommandContext(ctx, "pwsh", "-Command", script)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = os.Environ()
		return cmd.Run()

	case "linux", "darwin":
		shell := "bash"
		if _, err := exec.LookPath("bash"); err != nil {
			shell = "sh"
		}

		scriptPath := filepath.Join(tmpDir, "posix", "script")
		cmd := exec.CommandContext(ctx, shell, scriptPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = os.Environ()
		return cmd.Run()

	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func main() {
	if err := runGitClone(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

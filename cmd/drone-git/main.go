package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func copyDir(src, dst string) error {
	// Get all files in source directory
	files, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %v", src, err)
	}

	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dst, err)
	}

	for _, file := range files {
		srcPath := filepath.Join(src, file.Name())
		dstPath := filepath.Join(dst, file.Name())

		// Skip if it's a directory or not a script file
		if file.IsDir() || strings.HasSuffix(file.Name(), ".go") {
			continue
		}

		// Read source file
		content, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %v", srcPath, err)
		}

		// Write to destination with executable permissions
		if err := os.WriteFile(dstPath, content, 0755); err != nil {
			return fmt.Errorf("failed to write file %s: %v", dstPath, err)
		}
	}

	return nil
}

func runGitClone() error {
	// Get executable path to find script directories
	ex, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}
	baseDir := filepath.Dir(filepath.Dir(ex)) // Go up two levels from cmd/drone-git

	// Create a unique temporary subdirectory
	tmpDir, err := os.MkdirTemp("", "drone-git-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir) // Safe now as tmpDir is a unique subdirectory

	// Copy scripts to temp directory
	if err := copyDir(baseDir, tmpDir); err != nil {
		return err
	}

	ctx := context.Background()

	switch runtime.GOOS {
	case "windows":
		// From plugin.yml: pwsh.path: windows/clone.ps1
		script := fmt.Sprintf(
			"$ErrorActionPreference = 'Stop'; $ProgressPreference = 'SilentlyContinue'; %s",
			filepath.Join(tmpDir, "windows", "clone.ps1"))
		cmd := exec.CommandContext(ctx, "pwsh", "-Command", script)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = os.Environ()
		return cmd.Run()

	case "linux", "darwin":
		// From plugin.yml: bash.path: posix/script
		shell := "bash"
		if _, err := exec.LookPath("bash"); err != nil {
			shell = "sh"
		}

		cmd := exec.CommandContext(ctx, shell, filepath.Join(tmpDir, "posix", "script"))
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

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
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %v", src, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// If it's posix or windows directory, copy it
			if entry.Name() == "posix" || entry.Name() == "windows" {
				if err := os.MkdirAll(dstPath, 0755); err != nil {
					return fmt.Errorf("failed to create directory %s: %v", dstPath, err)
				}

				// Copy contents of the directory
				subEntries, err := os.ReadDir(srcPath)
				if err != nil {
					return fmt.Errorf("failed to read directory %s: %v", srcPath, err)
				}

				for _, subEntry := range subEntries {
					if subEntry.IsDir() || strings.HasSuffix(subEntry.Name(), ".go") {
						continue
					}

					srcFile := filepath.Join(srcPath, subEntry.Name())
					dstFile := filepath.Join(dstPath, subEntry.Name())

					content, err := os.ReadFile(srcFile)
					if err != nil {
						return fmt.Errorf("failed to read file %s: %v", srcFile, err)
					}

					if err := os.WriteFile(dstFile, content, 0755); err != nil {
						return fmt.Errorf("failed to write file %s: %v", dstFile, err)
					}
				}
			}
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

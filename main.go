package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/boyter/scc/v3/processor"
	"golang.org/x/exp/slog"
)

const mode os.FileMode = 0755

//go:embed posix/* windows/*
var scriptFS embed.FS // embedding both posix and windows directory scripts to be available to the binary

// CodeMetrics represents the code statistics collected by scc
type CodeMetrics struct {
	Lines      int64                      `json:"lines"`
	Code       int64                      `json:"code"`
	Comments   int64                      `json:"comments"`
	Blanks     int64                      `json:"blanks"`
	Complexity int64                      `json:"complexity"`
	Files      int64                      `json:"files"`
	Languages  map[string]LanguageMetrics `json:"languages"`
}

// LanguageMetrics represents metrics per programming language
type LanguageMetrics struct {
	Lines      int64 `json:"lines"`
	Code       int64 `json:"code"`
	Comments   int64 `json:"comments"`
	Blanks     int64 `json:"blanks"`
	Complexity int64 `json:"complexity"`
	Files      int64 `json:"files"`
}

// BuildToolData represents the complete build tool and metrics data written to file
type BuildToolData struct {
	// Existing fields from get-buildtool-lang script
	HarnessLang      string `json:"harness_lang"`
	HarnessBuildTool string `json:"harness_build_tool"`

	// New telemetry fields
	Timestamp     string      `json:"timestamp"`
	Repository    string      `json:"repository,omitempty"`
	RepositoryURL string      `json:"repository_url,omitempty"`
	Branch        string      `json:"branch,omitempty"`
	Commit        string      `json:"commit,omitempty"`
	BuildNumber   string      `json:"build_number,omitempty"`
	Metrics       CodeMetrics `json:"metrics"`
	PluginVersion string      `json:"plugin_version"`
}

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
		cmd := exec.CommandContext(ctx, "pwsh", "-Command", script)
		return runCmds([]*exec.Cmd{cmd}, os.Environ(), workdir, os.Stdout, os.Stderr)

	case "linux", "darwin":
		shell := "bash"
		if _, err := exec.LookPath("bash"); err != nil {
			shell = "sh"
		}

		scriptPath := filepath.Join(tmpDir, "posix", "script")
		cmd := exec.CommandContext(ctx, shell, scriptPath)
		return runCmds([]*exec.Cmd{cmd}, os.Environ(), workdir, os.Stdout, os.Stderr)

	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func runCmds(cmds []*exec.Cmd, env []string, workdir string,
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

// collectCodeMetrics analyzes code using scc Go library directly
func collectCodeMetrics(workdir string) (*CodeMetrics, error) {
	slog.Info("Collecting code metrics using scc library", "directory", workdir)

	// Set up timeout for analysis
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Configure scc processor with optimizations
	processor.DirFilePaths = []string{workdir}
	processor.Format = "json"
	processor.Files = false
	processor.Complexity = true  // Enable complexity calculations
	processor.Cocomo = false     // Disable COCOMO calculations for speed
	processor.Size = false       // Disable size calculations for speed
	processor.Duplicates = false // Disable duplicate detection for speed

	// Configure exclusions for performance
	processor.PathDenyList = []string{
		"node_modules", "vendor", "target", "build", ".git",
		"__pycache__", ".gradle", ".m2", "coverage", "dist",
		".svn", ".hg", "bin", "obj", "Debug", "Release",
	}
	processor.GitIgnore = true         // Respect .gitignore files
	processor.LargeByteCount = 1000000 // Skip files > 1MB
	processor.LargeLineCount = 40000   // Skip files > 40k lines

	// Channel to capture results
	done := make(chan error, 1)
	var results []processor.LanguageSummary

	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("scc analysis panicked: %v", r)
			}
		}()

		// Capture stdout to get JSON results
		oldStdout := os.Stdout
		r, w, err := os.Pipe()
		if err != nil {
			done <- fmt.Errorf("failed to create pipe: %v", err)
			return
		}
		os.Stdout = w

		// Run scc analysis
		processor.ConfigureLazy(true)
		processor.Process()

		// Restore stdout and read results
		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		io.Copy(&buf, r)

		// Parse JSON results
		if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
			done <- fmt.Errorf("failed to parse scc results: %v", err)
			return
		}

		done <- nil
	}()

	// Wait for completion or timeout
	select {
	case err := <-done:
		if err != nil {
			return nil, err
		}
	case <-ctx.Done():
		return nil, fmt.Errorf("scc analysis timed out after 30 seconds")
	}

	// Convert scc results to our telemetry format
	languages := make(map[string]LanguageMetrics)
	var totalLines, totalCode, totalComments, totalBlanks, totalComplexity, totalFiles int64

	for _, result := range results {
		if result.Name == "Total" {
			continue // Skip total row, we calculate our own
		}

		language := result.Name
		langMetrics := LanguageMetrics{
			Lines:      int64(result.Lines),
			Code:       int64(result.Code),
			Comments:   int64(result.Comment),
			Blanks:     int64(result.Blank),
			Complexity: int64(result.Complexity),
			Files:      int64(result.Count),
		}

		languages[language] = langMetrics

		// Update totals
		totalLines += langMetrics.Lines
		totalCode += langMetrics.Code
		totalComments += langMetrics.Comments
		totalBlanks += langMetrics.Blanks
		totalComplexity += langMetrics.Complexity
		totalFiles += langMetrics.Files
	}

	metrics := &CodeMetrics{
		Lines:      totalLines,
		Code:       totalCode,
		Comments:   totalComments,
		Blanks:     totalBlanks,
		Complexity: totalComplexity,
		Files:      totalFiles,
		Languages:  languages,
	}

	slog.Info("Code metrics collected using scc library",
		"total_lines", totalLines,
		"total_code", totalCode,
		"total_files", totalFiles,
		"languages", len(languages))

	return metrics, nil
}

// shouldSkipPath determines if a file path should be skipped during analysis
func shouldSkipPath(path string) bool {
	skipDirs := []string{
		"node_modules", "vendor", ".git", ".svn", ".hg",
		"target", "build", "dist", ".gradle", ".m2",
		"__pycache__", ".pytest_cache", ".tox",
		"coverage", ".nyc_output",
	}

	pathParts := strings.Split(path, string(filepath.Separator))
	for _, part := range pathParts {
		for _, skipDir := range skipDirs {
			if part == skipDir {
				return true
			}
		}
	}
	return false
}

// executeBuildToolScript executes the get-buildtool-lang script and lets it write to file
func executeBuildToolScript(workdir string) error {
	buildToolFile := os.Getenv("PLUGIN_BUILD_TOOL_FILE")
	if buildToolFile == "" {
		return nil // No file specified, nothing to do
	}

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		// Execute PowerShell script
		scriptPath := filepath.Join(workdir, "windows", "get-buildtool-lang.ps1")
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			// Extract script from embedded files first
			if err := writeScriptsToTemp(workdir); err != nil {
				return fmt.Errorf("failed to extract Windows scripts: %v", err)
			}
		}
		cmd = exec.Command("pwsh", "-File", scriptPath, workdir)

	case "linux", "darwin":
		// Execute shell script
		scriptPath := filepath.Join(workdir, "posix", "get-buildtool-lang")
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			// Extract script from embedded files first
			if err := writeScriptsToTemp(workdir); err != nil {
				return fmt.Errorf("failed to extract POSIX scripts: %v", err)
			}
		}

		shell := "bash"
		if _, err := exec.LookPath("bash"); err != nil {
			shell = "sh"
		}
		cmd = exec.Command(shell, scriptPath, workdir)

	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	cmd.Dir = workdir
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		slog.Warn("Build tool script execution failed", "error", err)
		return err
	}

	slog.Debug("Build tool script executed successfully")
	return nil
}

// collectAndWriteMetrics collects code metrics and writes complete build tool file
func collectAndWriteMetrics(workdir string) {
	buildToolFile := os.Getenv("PLUGIN_BUILD_TOOL_FILE")
	if buildToolFile == "" {
		slog.Debug("No PLUGIN_BUILD_TOOL_FILE specified, skipping metrics collection")
		return
	}

	// Collect code metrics using scc library
	metrics, err := collectCodeMetrics(workdir)
	if err != nil {
		slog.Warn("Failed to collect code metrics", "error", err)
		// Continue with empty metrics rather than failing
		metrics = &CodeMetrics{
			Lines:      0,
			Code:       0,
			Comments:   0,
			Blanks:     0,
			Complexity: 0,
			Files:      0,
			Languages:  make(map[string]LanguageMetrics),
		}
	}

	// Execute existing build tool script first
	if err := executeBuildToolScript(workdir); err != nil {
		slog.Warn("Build tool script failed, continuing with empty values", "error", err)
	}

	// Read the output from the script
	existingData := make(map[string]interface{})
	if data, err := os.ReadFile(buildToolFile); err == nil {
		if err := json.Unmarshal(data, &existingData); err != nil {
			slog.Warn("Failed to parse script output", "error", err)
		}
	}

	// Extract values from script output
	harnessLang, _ := existingData["harness_lang"].(string)
	harnessBuildTool, _ := existingData["harness_build_tool"].(string)

	// Prepare complete build tool data
	buildToolData := &BuildToolData{
		// Existing fields (compatibility with get-buildtool-lang)
		HarnessLang:      harnessLang,
		HarnessBuildTool: harnessBuildTool,

		// New telemetry fields
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Repository:    os.Getenv("DRONE_REPO"),
		RepositoryURL: getRepositoryURL(),
		Branch:        os.Getenv("DRONE_BRANCH"),
		Commit:        os.Getenv("DRONE_COMMIT"),
		BuildNumber:   os.Getenv("DRONE_BUILD_NUMBER"),
		Metrics:       *metrics,
	}

	// Write complete file once
	if err := writeBuildToolFile(buildToolData); err != nil {
		slog.Error("Failed to write build tool file", "error", err)
	}
}

// writeBuildToolFile writes build tool and metrics data to the specified file
func writeBuildToolFile(data *BuildToolData) error {
	buildToolFile := os.Getenv("PLUGIN_BUILD_TOOL_FILE")
	if buildToolFile == "" {
		slog.Debug("No PLUGIN_BUILD_TOOL_FILE specified, skipping file write")
		return nil
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal build tool data: %v", err)
	}

	if err := os.WriteFile(buildToolFile, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write build tool file %s: %v", buildToolFile, err)
	}

	slog.Info("Build tool and metrics data written", "file", buildToolFile, "size", len(jsonData))
	return nil
}

// getRepositoryURL extracts the repository URL from Drone environment variables
func getRepositoryURL() string {
	// Primary: DRONE_REMOTE_URL is the actual git remote URL used by clone scripts
	if url := os.Getenv("DRONE_REMOTE_URL"); url != "" {
		return url
	}

	return "" // No URL available
}

func main() {
	// Get current working directory for telemetry
	workdir, err := os.Getwd()
	if err != nil {
		slog.Error("cannot get workdir", "error", err)
		os.Exit(1)
	}

	// Run git clone
	if err := runGitClone(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Collect code metrics and write complete build tool file
	collectAndWriteMetrics(workdir)
}

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

// Version information (set during build)
var (
	version = "dev"
)

//go:embed posix/* windows/*
var scriptFS embed.FS // embedding both posix and windows directory scripts to be available to the binary

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
	Repository      string      `json:"repository,omitempty"`
	BuildEvent      string      `json:"build_event,omitempty"`
	BuildEventValue string      `json:"build_event_value,omitempty"`
	Metrics         CodeMetrics `json:"metrics"`
	PluginVersion   string      `json:"plugin_version"`
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

// Global temp directory for script storage (shared between functions)
var globalTmpDir string

// cleanupTempDir safely removes the temp directory if it exists
func cleanupTempDir() {
	if globalTmpDir != "" {
		if err := os.RemoveAll(globalTmpDir); err != nil {
			slog.Warn("Failed to cleanup temp directory", "dir", globalTmpDir, "error", err)
		} else {
			slog.Debug("Cleaned up temp directory", "dir", globalTmpDir)
		}
		globalTmpDir = ""
	}
}

func runGitClone() error {
	var err error
	// Create a unique temporary subdirectory (keep alive for script reuse in metrics)
	globalTmpDir, err = os.MkdirTemp("", "drone-git-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %v", err)
	}

	if err := writeScriptsToTemp(globalTmpDir); err != nil {
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
		scriptPath := filepath.Join(globalTmpDir, "windows", "clone.ps1")
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

		scriptPath := filepath.Join(globalTmpDir, "posix", "script")
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

func collectCodeMetrics(workdir string) (*CodeMetrics, error) {

	// Set up timeout for analysis
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Configure scc processor with optimizations
	processor.DirFilePaths = []string{workdir}
	processor.Format = "json"
	processor.Files = false
	processor.Complexity = true  // Disable complexity calculations
	processor.Cocomo = true      // Disable COCOMO calculations for speed
	processor.Size = true        // Disable size calculations for speed
	processor.Duplicates = false // Disable duplicate detection for speed

	// Configure exclusions for performance
	processor.PathDenyList = []string{
		"node_modules", "vendor", "target", "build", ".git",
		"__pycache__", ".gradle", ".m2", "coverage", "dist",
		".svn", ".hg", "bin", "obj", "Debug", "Release",
	}
	processor.GitIgnore = false
	processor.NoLarge = true
	processor.LargeByteCount = 1000000 // Skip files > 1MB
	processor.LargeLineCount = 40000   // Skip files > 40k lines

	// Channel to capture results
	done := make(chan error, 1)
	var results []processor.LanguageSummary

	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("analysis panicked: %v", r)
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
			done <- fmt.Errorf("failed to parse analysis results: %v", err)
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
		return nil, fmt.Errorf("analysis timed out after 5 seconds")
	}

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

	return metrics, nil
}

// executeBuildToolScript executes the get-buildtool-lang script from temp directory
func executeBuildToolScript(workdir string) error {
	buildToolFile := os.Getenv("PLUGIN_BUILD_TOOL_FILE")
	if buildToolFile == "" {
		return nil // No file specified, nothing to do
	}

	// Scripts are already extracted to globalTmpDir by runGitClone()
	if globalTmpDir == "" {
		return fmt.Errorf("temp directory not initialized - runGitClone() must be called first")
	}

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		// Execute PowerShell script from temp directory (no workspace pollution)
		scriptPath := filepath.Join(globalTmpDir, "windows", "get-buildtool-lang.ps1")
		cmd = exec.Command("pwsh", "-File", scriptPath, workdir)

	case "linux", "darwin":
		// Execute shell script from temp directory (no workspace pollution)
		scriptPath := filepath.Join(globalTmpDir, "posix", "get-buildtool-lang")

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

// tryCollectAndWriteMetrics attempts to collect code metrics and write build tool file
func tryCollectAndWriteMetrics(workdir string) error {
	buildToolFile := os.Getenv("PLUGIN_BUILD_TOOL_FILE")
	if buildToolFile == "" {
		slog.Debug("No PLUGIN_BUILD_TOOL_FILE specified, skipping metrics collection")
		return nil
	}

	// Respect CI_DISABLE_TELEMETRY flag - disables everything (same as original script condition)
	if os.Getenv("CI_DISABLE_TELEMETRY") != "" {
		slog.Debug("All telemetry disabled via CI_DISABLE_TELEMETRY, skipping collection")
		return nil
	}

	// Skip metrics collection if temp directory not available (runGitClone not called)
	if globalTmpDir == "" {
		slog.Debug("No temp directory available, skipping metrics collection")
		return nil
	}

	// Always execute build tool script first (basic harness_lang, harness_build_tool data)
	if err := executeBuildToolScript(workdir); err != nil {
		slog.Warn("Build tool script failed, continuing with empty values", "error", err)
	}

	// Read the build tool script output
	existingData := make(map[string]interface{})
	if data, err := os.ReadFile(buildToolFile); err == nil {
		if err := json.Unmarshal(data, &existingData); err != nil {
			slog.Warn("Failed to parse script output", "error", err)
		}
	}

	// Extract values from script output
	harnessLang, _ := existingData["harness_lang"].(string)
	harnessBuildTool, _ := existingData["harness_build_tool"].(string)

	// Collect SCC metrics only if not specifically disabled
	var metrics *CodeMetrics
	if os.Getenv("DISABLE_SCC_METRICS") != "" {
		slog.Debug("Metrics disabled, using empty metrics")
		metrics = &CodeMetrics{
			Lines:      0,
			Code:       0,
			Comments:   0,
			Blanks:     0,
			Complexity: 0,
			Files:      0,
			Languages:  make(map[string]LanguageMetrics),
		}
	} else {
		var err error
		metrics, err = collectCodeMetrics(workdir)
		if err != nil {
			slog.Warn("Failed to collect metrics", "error", err)
			// Use empty metrics if scc fails
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
	}

	// Get build event and value
	buildEvent, buildEventValue := getBuildEventInfo()

	// Prepare complete build tool data (always includes build tool info)
	buildToolData := &BuildToolData{
		// Always include build tool fields (ensures backward compatibility)
		HarnessLang:      harnessLang,
		HarnessBuildTool: harnessBuildTool,

		// Context fields
		Repository:      getRepositoryURL(),
		BuildEvent:      buildEvent,
		BuildEventValue: buildEventValue,
		Metrics:         *metrics,
		PluginVersion:   getPluginVersion(),
	}

	// Always write the file (ensures build tool data flows through)
	if err := writeBuildToolFile(buildToolData); err != nil {
		return fmt.Errorf("failed to write build tool file: %v", err)
	}

	return nil
}

// collectAndWriteMetrics is a wrapper for backward compatibility (used by tests)
func collectAndWriteMetrics(workdir string) {
	// For tests: initialize temp directory if needed (production skips if not available)
	if globalTmpDir == "" {
		var err error
		globalTmpDir, err = os.MkdirTemp("", "drone-git-test-*")
		if err != nil {
			slog.Warn("Failed to create temp directory for test", "error", err)
			return
		}
		if err := writeScriptsToTemp(globalTmpDir); err != nil {
			slog.Warn("Failed to extract scripts for test", "error", err)
			return
		}
		slog.Debug("Initialized temp directory for test", "dir", globalTmpDir)
		defer cleanupTempDir() // Cleanup after test
	}

	if err := tryCollectAndWriteMetrics(workdir); err != nil {
		slog.Warn("Metrics collection failed", "error", err)
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

// getPluginVersion returns the plugin version from various sources
func getPluginVersion() string {
	// 1. Check if version was set during build (via -ldflags)
	if version != "dev" && version != "" {
		return version
	}
	// 3. Fallback to default
	return "1.0.0"
}

// getBuildEventInfo determines build event type and value based on available Drone environment variables
func getBuildEventInfo() (string, string) {
	// Use DRONE_BUILD_EVENT as primary source (used in existing scripts)
	buildEvent := os.Getenv("DRONE_BUILD_EVENT")

	switch buildEvent {
	case "tag":
		// TAG build - use DRONE_TAG as value
		if droneTag := os.Getenv("DRONE_TAG"); droneTag != "" {
			return "tag", droneTag
		}
		return "tag", ""

	case "pull_request":
		// PR build - use source branch if available
		if sourceBranch := os.Getenv("DRONE_SOURCE_BRANCH"); sourceBranch != "" {
			return "pull_request", sourceBranch
		}
		return "pull_request", ""

	case "push":
		// Push to branch - try to get branch info
		if commitBranch := os.Getenv("DRONE_COMMIT_BRANCH"); commitBranch != "" {
			return "branch", commitBranch
		}
		return "push", ""

	default:
		// Fallback: Check for tag via DRONE_TAG even if DRONE_BUILD_EVENT not set
		if droneTag := os.Getenv("DRONE_TAG"); droneTag != "" {
			return "tag", droneTag
		}

		// Fallback: Check for commit via DRONE_COMMIT_SHA
		if commitSha := os.Getenv("DRONE_COMMIT_SHA"); commitSha != "" {
			return "commit", commitSha
		}
	}

	// Unknown build type
	return "", ""
}

// getWorkspaceDirectory returns the directory to analyze (DRONE_WORKSPACE preferred, current dir as fallback)
func getWorkspaceDirectory() (string, error) {
	// 1. Use DRONE_WORKSPACE if set (where repository gets cloned)
	if workspace := os.Getenv("DRONE_WORKSPACE"); workspace != "" {
		slog.Debug("Using DRONE_WORKSPACE for analysis", "directory", workspace)
		return workspace, nil
	}

	// 2. Fallback to current working directory
	workdir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot get working directory: %v", err)
	}

	slog.Debug("Using current directory for analysis", "directory", workdir)
	return workdir, nil
}

func main() {
	// Ensure temp directory cleanup happens regardless of execution path
	defer cleanupTempDir()

	// Run git clone first (core functionality - can fail the step)
	if err := runGitClone(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		cleanupTempDir() // Manual cleanup before exit
		os.Exit(1)       // Core git functionality failure - should fail step
	}

	// Git clone succeeded - now attempt analytics (optional)
	// Get workspace directory for analysis
	workdir, err := getWorkspaceDirectory()
	if err != nil {
		slog.Warn("Cannot get workspace directory for analytics, skipping metrics collection", "error", err)
		return // Analytics failure - don't fail the step, just skip
	}

	// Collect code metrics and write complete build tool file
	// Note: Analytics failures should not fail the step
	if err := tryCollectAndWriteMetrics(workdir); err != nil {
		slog.Warn("Metrics collection failed but continuing (analytics only)", "error", err)
		// Continue - don't fail the step for analytics issues
	}
}

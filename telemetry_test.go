package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectCodeMetrics(t *testing.T) {
	// Create a temporary directory with test files
	tmpDir, err := os.MkdirTemp("", "drone-git-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test files
	testFiles := map[string]string{
		"main.go": `package main

import "fmt"

// This is a comment
func main() {
	fmt.Println("Hello, World!")
}
`,
		"utils.js": `// JavaScript utility functions
function add(a, b) {
    return a + b;
}

function multiply(a, b) {
    // Multiply two numbers
    return a * b;
}
`,
		"README.md": `# Test Project

This is a test project for telemetry.
`,
	}

	for filename, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(content), 0644)
		require.NoError(t, err)
	}

	// Test metrics collection
	metrics, err := collectCodeMetrics(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	// Verify total metrics
	assert.Greater(t, metrics.Lines, int64(0), "Should have collected lines")
	assert.Greater(t, metrics.Code, int64(0), "Should have collected code lines")
	assert.Greater(t, metrics.Files, int64(0), "Should have collected files")

	// Verify language breakdown
	assert.Contains(t, metrics.Languages, "Go", "Should detect Go language")
	assert.Contains(t, metrics.Languages, "JavaScript", "Should detect JavaScript language")

	// Check Go metrics
	goMetrics := metrics.Languages["Go"]
	assert.Greater(t, goMetrics.Lines, int64(0), "Go should have lines")
	assert.Greater(t, goMetrics.Code, int64(0), "Go should have code lines")
	assert.Equal(t, int64(1), goMetrics.Files, "Should have 1 Go file")

	// Check JavaScript metrics
	jsMetrics := metrics.Languages["JavaScript"]
	assert.Greater(t, jsMetrics.Lines, int64(0), "JavaScript should have lines")
	assert.Greater(t, jsMetrics.Code, int64(0), "JavaScript should have code lines")
	assert.Equal(t, int64(1), jsMetrics.Files, "Should have 1 JavaScript file")
}

func TestCollectAndWriteMetrics_NoFile(t *testing.T) {
	// Ensure no build tool file is set
	os.Unsetenv("PLUGIN_BUILD_TOOL_FILE")

	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "drone-git-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// This should not panic or fail when no file is specified
	collectAndWriteMetrics(tmpDir)
}

func TestCollectAndWriteMetrics_TelemetryDisabled(t *testing.T) {
	// Set up build tool file but disable telemetry
	tmpDir, err := os.MkdirTemp("", "drone-git-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	buildToolFile := filepath.Join(tmpDir, "build-tool-info.json")
	os.Setenv("PLUGIN_BUILD_TOOL_FILE", buildToolFile)
	os.Setenv("CI_DISABLE_TELEMETRY", "true")
	defer func() {
		os.Unsetenv("PLUGIN_BUILD_TOOL_FILE")
		os.Unsetenv("CI_DISABLE_TELEMETRY")
	}()

	// Function should skip metrics collection when telemetry is disabled
	collectAndWriteMetrics(tmpDir)

	// File should not be created when telemetry is disabled
	assert.NoFileExists(t, buildToolFile, "Build tool file should not be created when telemetry disabled")
}

func TestCollectAndWriteMetrics_SCCDisabled(t *testing.T) {
	// Create test repo structure
	tmpDir, err := os.MkdirTemp("", "drone-git-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	testFiles := map[string]string{
		"main.go":      `package main\nfunc main() {}`,
		"app.js":       `console.log("test");`,
		"go.mod":       `module test`,
		"package.json": `{"name": "test"}`,
	}

	for filename, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(content), 0644)
		require.NoError(t, err)
	}

	// Set up build tool file and disable only SCC metrics
	buildToolFile := filepath.Join(tmpDir, "build-tool-info.json")
	os.Setenv("PLUGIN_BUILD_TOOL_FILE", buildToolFile)
	os.Setenv("DISABLE_SCC_METRICS", "true")
	os.Setenv("DRONE_REPO", "test/repo")
	os.Setenv("DRONE_REMOTE_URL", "https://github.com/test/repo.git")
	defer func() {
		os.Unsetenv("PLUGIN_BUILD_TOOL_FILE")
		os.Unsetenv("DISABLE_SCC_METRICS")
		os.Unsetenv("DRONE_REPO")
		os.Unsetenv("DRONE_REMOTE_URL")
	}()

	// Run the function
	collectAndWriteMetrics(tmpDir)

	// Verify file was still created (build tool data should flow through)
	assert.FileExists(t, buildToolFile, "Build tool file should be created even with SCC disabled")

	// Read and verify content
	data, err := os.ReadFile(buildToolFile)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	// Verify build tool data is present (backward compatibility)
	assert.Contains(t, result, "harness_lang", "Should contain harness_lang even with SCC disabled")
	assert.Contains(t, result, "harness_build_tool", "Should contain harness_build_tool even with SCC disabled")
	assert.Contains(t, result, "repository", "Should contain repository info")

	// Verify SCC metrics are empty/zero when disabled
	assert.Contains(t, result, "metrics", "Should contain metrics field")
	metrics, ok := result["metrics"].(map[string]interface{})
	assert.True(t, ok, "Metrics should be a map")

	// SCC metrics should be zero when disabled
	assert.Equal(t, float64(0), metrics["lines"], "Lines should be 0 when SCC disabled")
	assert.Equal(t, float64(0), metrics["code"], "Code should be 0 when SCC disabled")
	assert.Equal(t, float64(0), metrics["files"], "Files should be 0 when SCC disabled")
}

func TestCollectAndWriteMetrics_WithFile(t *testing.T) {
	// Create a temporary directory with test files
	tmpDir, err := os.MkdirTemp("", "drone-git-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test repo structure
	testFiles := map[string]string{
		"main.go":      `package main\nfunc main() {}`,
		"app.js":       `console.log("test");`,
		"go.mod":       `module test`,
		"package.json": `{"name": "test"}`,
	}

	for filename, content := range testFiles {
		err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(content), 0644)
		require.NoError(t, err)
	}

	// Set up build tool file
	buildToolFile := filepath.Join(tmpDir, "build-tool-info.json")
	os.Setenv("PLUGIN_BUILD_TOOL_FILE", buildToolFile)
	defer os.Unsetenv("PLUGIN_BUILD_TOOL_FILE")

	// Set up Drone environment variables
	os.Setenv("DRONE_REPO", "test/repo")
	os.Setenv("DRONE_REMOTE_URL", "https://github.com/test/repo.git")
	os.Setenv("DRONE_BRANCH", "main")
	os.Setenv("DRONE_COMMIT", "abc123")
	defer func() {
		os.Unsetenv("DRONE_REPO")
		os.Unsetenv("DRONE_REMOTE_URL")
		os.Unsetenv("DRONE_BRANCH")
		os.Unsetenv("DRONE_COMMIT")
	}()

	// Run the function
	collectAndWriteMetrics(tmpDir)

	// Verify file was created
	assert.FileExists(t, buildToolFile, "Build tool file should be created")

	// Read and verify content
	data, err := os.ReadFile(buildToolFile)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	// Verify structure includes both original fields and new metrics
	assert.Contains(t, result, "harness_lang", "Should contain harness_lang")
	assert.Contains(t, result, "harness_build_tool", "Should contain harness_build_tool")
	assert.Contains(t, result, "timestamp", "Should contain timestamp")
	assert.Contains(t, result, "repository", "Should contain repository")
	assert.Contains(t, result, "metrics", "Should contain metrics")

	// Verify Drone environment variables were captured
	assert.Equal(t, "https://github.com/test/repo.git", result["repository"], "Should capture DRONE_REMOTE_URL as repository")
	assert.Equal(t, "main", result["branch"], "Should capture DRONE_BRANCH")
	assert.Equal(t, "abc123", result["commit"], "Should capture DRONE_COMMIT")
}

func TestBuildToolDataStructure(t *testing.T) {
	// Test BuildToolData marshaling
	metrics := CodeMetrics{
		Lines:      100,
		Code:       75,
		Comments:   15,
		Blanks:     10,
		Complexity: 25,
		Files:      5,
		Languages: map[string]LanguageMetrics{
			"Go": {
				Lines:      100,
				Code:       75,
				Comments:   15,
				Blanks:     10,
				Complexity: 25,
				Files:      5,
			},
		},
	}

	buildToolData := BuildToolData{
		HarnessLang:      "Go,JavaScript",
		HarnessBuildTool: "Go",
		Timestamp:        "2025-01-15T10:30:00Z",
		Repository:       "https://github.com/test/repo.git",
		Branch:           "main",
		Commit:           "abc123",
		BuildNumber:      "42",
		Metrics:          metrics,
		PluginVersion:    "1.0.0",
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(buildToolData)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "harness_lang", "Should contain harness_lang field")
	assert.Contains(t, string(jsonData), "harness_build_tool", "Should contain harness_build_tool field")
	assert.Contains(t, string(jsonData), "https://github.com/test/repo.git", "Should contain repository URL")
	assert.Contains(t, string(jsonData), "main", "Should contain branch name")
	assert.Contains(t, string(jsonData), "abc123", "Should contain commit")
}

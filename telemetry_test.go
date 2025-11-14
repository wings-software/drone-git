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

func TestShouldSkipPath(t *testing.T) {
	testCases := []struct {
		path     string
		expected bool
	}{
		{"src/main.go", false},
		{"node_modules/package/index.js", true},
		{"vendor/dependency/file.go", true},
		{".git/config", true},
		{"build/output.jar", true},
		{"src/components/Button.tsx", false},
		{"__pycache__/module.pyc", true},
		{"coverage/report.html", true},
		{"normal/path/file.py", false},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			result := shouldSkipPath(tc.path)
			assert.Equal(t, tc.expected, result, "Path %s should skip: %v", tc.path, tc.expected)
		})
	}
}

func TestCollectAndWriteMetrics_NoFile(t *testing.T) {
	// Ensure no build tool file is set
	os.Unsetenv("PLUGIN_BUILD_TOOL_FILE")

	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "drone-git-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// This should not panic or fail when no file is specified
	collectAndWriteMetricsSync(tmpDir)
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
		Repository:       "test/repo",
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
	assert.Contains(t, string(jsonData), "test/repo", "Should contain repository name")
	assert.Contains(t, string(jsonData), "main", "Should contain branch name")
	assert.Contains(t, string(jsonData), "abc123", "Should contain commit")
}

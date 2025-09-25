// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package llmagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elastic/elastic-package/internal/packages"
)

func TestNewDocumentationAgent(t *testing.T) {
	tempDir := t.TempDir()

	// Create a basic package manifest
	manifestContent := `format_version: 3.0.0
name: test-package
title: Test Package
description: A test package for documentation
version: 1.0.0
license: basic
type: integration`

	manifestPath := filepath.Join(tempDir, "manifest.yml")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("Failed to create manifest: %v", err)
	}

	provider := NewMockLLMProvider(NewPackageNoREADME)

	agent, err := NewDocumentationAgent(provider, tempDir)

	if err != nil {
		t.Fatalf("NewDocumentationAgent failed: %v", err)
	}

	if agent == nil {
		t.Fatal("NewDocumentationAgent should return a non-nil agent")
	}

	if agent.packageRoot != tempDir {
		t.Errorf("Expected package root '%s', got '%s'", tempDir, agent.packageRoot)
	}

	if agent.templateContent == "" {
		t.Error("Template content should not be empty")
	}

	if agent.agent == nil {
		t.Error("Internal agent should not be nil")
	}

	if agent.originalReadmeContent != nil {
		t.Error("Original README content should be nil for new agent")
	}
}

func TestDocumentationAgent_BuildInitialPrompt(t *testing.T) {
	tempDir := t.TempDir()
	createTestPackageStructure(t, tempDir, "test-package", false, "")

	provider := NewMockLLMProvider(NewPackageNoREADME)
	agent, err := NewDocumentationAgent(provider, tempDir)
	if err != nil {
		t.Fatalf("NewDocumentationAgent failed: %v", err)
	}

	manifest := &packages.PackageManifest{
		Name:        "test-package",
		Title:       "Test Package",
		Type:        "integration",
		Version:     "1.0.0",
		Description: "A test integration package",
	}

	prompt := agent.buildInitialPrompt(manifest)

	// Verify prompt contains all expected elements
	expectedElements := []string{
		"test-package",
		"Test Package",
		"integration",
		"1.0.0",
		"A test integration package",
		"Initial Analysis",
		"Please begin. Start with the \"Initial Analysis\" step",
		"Template to Follow:",
		"get_example_readme",
	}

	for _, element := range expectedElements {
		if !strings.Contains(prompt, element) {
			t.Errorf("Prompt should contain '%s'", element)
		}
	}

	// Verify prompt structure
	if !strings.Contains(prompt, "Core Task:") {
		t.Error("Prompt should contain core task section")
	}

	if !strings.Contains(prompt, "Critical Directives") {
		t.Error("Prompt should contain critical directives")
	}

	if !strings.Contains(prompt, "Step-by-Step Process:") {
		t.Error("Prompt should contain step-by-step process")
	}
}

func TestDocumentationAgent_BackupAndRestoreReadme(t *testing.T) {
	tempDir := t.TempDir()

	originalContent := "# Original README\n\nThis is the original content."
	createTestPackageStructure(t, tempDir, "test-package", true, originalContent)

	provider := NewMockLLMProvider(NewPackageNoREADME)
	agent, err := NewDocumentationAgent(provider, tempDir)
	if err != nil {
		t.Fatalf("NewDocumentationAgent failed: %v", err)
	}

	// Test backup
	agent.backupOriginalReadme()

	if agent.originalReadmeContent == nil {
		t.Error("Should have backed up original README content")
	}

	if *agent.originalReadmeContent != originalContent {
		t.Errorf("Backed up content should match original.\nExpected: %s\nGot: %s",
			originalContent, *agent.originalReadmeContent)
	}

	// Modify the README file
	readmePath := filepath.Join(tempDir, "_dev", "build", "docs", "README.md")
	newContent := "# Modified README\n\nThis is modified content."
	if err := os.WriteFile(readmePath, []byte(newContent), 0644); err != nil {
		t.Fatalf("Failed to modify README: %v", err)
	}

	// Test restore
	agent.restoreOriginalReadme()

	// Verify content was restored
	restoredContent, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("Failed to read restored README: %v", err)
	}

	if string(restoredContent) != originalContent {
		t.Errorf("Restored content should match original.\nExpected: %s\nGot: %s",
			originalContent, string(restoredContent))
	}
}

func TestDocumentationAgent_ValidatePreservedSections(t *testing.T) {
	tempDir := t.TempDir()

	provider := NewMockLLMProvider(NewPackageNoREADME)
	agent, err := NewDocumentationAgent(provider, tempDir)
	if err != nil {
		t.Fatalf("NewDocumentationAgent failed: %v", err)
	}

	originalContent := `# Package

Some content here.

<!-- HUMAN-EDITED START -->
Important user content.
<!-- HUMAN-EDITED END -->

More content.

<!-- PRESERVE START -->
Another preserved section.
<!-- PRESERVE END -->

Final content.
`

	tests := []struct {
		name         string
		newContent   string
		wantWarnings int
	}{
		{
			name: "all sections preserved",
			newContent: `# Updated Package

Updated content.

<!-- HUMAN-EDITED START -->
Important user content.
<!-- HUMAN-EDITED END -->

More updated content.

<!-- PRESERVE START -->
Another preserved section.
<!-- PRESERVE END -->

Final updated content.
`,
			wantWarnings: 0,
		},
		{
			name: "one section missing",
			newContent: `# Updated Package

Updated content without preserved sections.

<!-- HUMAN-EDITED START -->
Important user content.
<!-- HUMAN-EDITED END -->

Final content.
`,
			wantWarnings: 1,
		},
		{
			name: "all sections missing",
			newContent: `# Completely New Package

No preserved content at all.
`,
			wantWarnings: 2,
		},
		{
			name: "content modified but markers present",
			newContent: `# Updated Package

<!-- HUMAN-EDITED START -->
Modified user content.
<!-- HUMAN-EDITED END -->

<!-- PRESERVE START -->
Modified preserved section.
<!-- PRESERVE END -->
`,
			wantWarnings: 2, // Content changed, so not preserved
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := agent.validatePreservedSections(originalContent, tt.newContent)

			if len(warnings) != tt.wantWarnings {
				t.Errorf("Expected %d warnings, got %d: %v",
					tt.wantWarnings, len(warnings), warnings)
			}
		})
	}
}

func TestDocumentationAgent_ExtractPreservedSections(t *testing.T) {
	tempDir := t.TempDir()

	provider := NewMockLLMProvider(NewPackageNoREADME)
	agent, err := NewDocumentationAgent(provider, tempDir)
	if err != nil {
		t.Fatalf("NewDocumentationAgent failed: %v", err)
	}

	content := `# Package

Some content.

<!-- HUMAN-EDITED START -->
First human section.
<!-- HUMAN-EDITED END -->

More content.

<!-- HUMAN-EDITED START -->
Second human section.
<!-- HUMAN-EDITED END -->

<!-- PRESERVE START -->
Preserved section.
<!-- PRESERVE END -->

Final content.
`

	sections := agent.extractPreservedSections(content)

	expectedSections := map[string]bool{
		"HUMAN-EDITED-1": true,
		"HUMAN-EDITED-2": true,
		"PRESERVE-1":     true,
	}

	if len(sections) != len(expectedSections) {
		t.Errorf("Expected %d sections, got %d", len(expectedSections), len(sections))
	}

	for key := range sections {
		if !expectedSections[key] {
			t.Errorf("Unexpected section key: %s", key)
		}
	}

	// Verify content extraction
	if !strings.Contains(sections["HUMAN-EDITED-1"], "First human section.") {
		t.Error("Should extract first human-edited section")
	}

	if !strings.Contains(sections["HUMAN-EDITED-2"], "Second human section.") {
		t.Error("Should extract second human-edited section")
	}

	if !strings.Contains(sections["PRESERVE-1"], "Preserved section.") {
		t.Error("Should extract preserved section")
	}
}

func TestDocumentationAgent_CheckReadmeUpdated(t *testing.T) {
	tempDir := t.TempDir()

	provider := NewMockLLMProvider(NewPackageNoREADME)
	agent, err := NewDocumentationAgent(provider, tempDir)
	if err != nil {
		t.Fatalf("NewDocumentationAgent failed: %v", err)
	}

	// No original content, no README file
	if agent.checkReadmeUpdated() {
		t.Error("Should not be updated when no file exists")
	}

	// Create README file
	readmePath := filepath.Join(tempDir, "_dev", "build", "docs", "README.md")
	if err := os.MkdirAll(filepath.Dir(readmePath), 0755); err != nil {
		t.Fatalf("Failed to create dirs: %v", err)
	}

	content := "# New README"
	if err := os.WriteFile(readmePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write README: %v", err)
	}

	// No original content, but file exists
	if !agent.checkReadmeUpdated() {
		t.Error("Should be updated when file exists and no original")
	}

	// Set original content same as current
	agent.originalReadmeContent = &content
	if agent.checkReadmeUpdated() {
		t.Error("Should not be updated when content is same")
	}

	// Change content
	newContent := "# Updated README"
	if err := os.WriteFile(readmePath, []byte(newContent), 0644); err != nil {
		t.Fatalf("Failed to update README: %v", err)
	}

	if !agent.checkReadmeUpdated() {
		t.Error("Should be updated when content changed")
	}
}

func TestDocumentationAgent_ErrorResponseDetection(t *testing.T) {
	tests := []struct {
		name    string
		content string
		isError bool
	}{
		{
			name:    "normal response",
			content: "I have successfully analyzed the package and created the documentation.",
			isError: false,
		},
		{
			name:    "error response",
			content: "I encountered an error while trying to read the manifest file.",
			isError: false, // This is now treated as LLM confusion, not a real error
		},
		{
			name:    "unable to complete",
			content: "I'm unable to complete this task due to missing information.",
			isError: true,
		},
		{
			name:    "task failed",
			content: "I failed to generate the documentation due to network issues.",
			isError: true,
		},
		{
			name:    "max iterations",
			content: "Task did not complete within maximum iterations.",
			isError: true,
		},
		{
			name:    "case insensitive",
			content: "SOMETHING WENT WRONG during the process.",
			isError: true,
		},
		{
			name:    "partial match",
			content: "The process is working fine, no errors encountered.",
			isError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isErrorResponse(tt.content)
			if result != tt.isError {
				t.Errorf("isErrorResponse(%q) = %t, want %t", tt.content, result, tt.isError)
			}
		})
	}
}

// Helper function to create test package structure
func createTestPackageStructure(t testing.TB, packageRoot, packageName string, hasREADME bool, readmeContent string) {
	t.Helper()

	// Create manifest
	manifestContent := fmt.Sprintf(`format_version: 3.0.0
name: %s
title: %s
description: A test package for documentation
version: 1.0.0
license: basic
type: integration`, packageName, strings.Title(strings.ReplaceAll(packageName, "-", " ")))

	manifestPath := filepath.Join(packageRoot, "manifest.yml")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("Failed to create manifest: %v", err)
	}

	// Create data stream structure
	dataStreamDir := filepath.Join(packageRoot, "data_stream", "logs")
	if err := os.MkdirAll(dataStreamDir, 0755); err != nil {
		t.Fatalf("Failed to create data stream dir: %v", err)
	}

	dsManifest := `title: Log data stream
type: logs
description: Collects log data`

	dsManifestPath := filepath.Join(dataStreamDir, "manifest.yml")
	if err := os.WriteFile(dsManifestPath, []byte(dsManifest), 0644); err != nil {
		t.Fatalf("Failed to create data stream manifest: %v", err)
	}

	// Create README if requested
	if hasREADME {
		readmeDir := filepath.Join(packageRoot, "_dev", "build", "docs")
		if err := os.MkdirAll(readmeDir, 0755); err != nil {
			t.Fatalf("Failed to create README dir: %v", err)
		}

		readmePath := filepath.Join(readmeDir, "README.md")
		if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
			t.Fatalf("Failed to create README: %v", err)
		}
	}
}

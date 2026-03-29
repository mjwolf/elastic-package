// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEnsureDocsStructureEnforced(t *testing.T) {
	tests := []struct {
		name           string
		existingConfig string
		expectModified bool
		wantErr        bool
	}{
		{
			name:           "creates new file",
			existingConfig: "",
			expectModified: true,
			wantErr:        false,
		},
		{
			name: "adds docs_structure_enforced to existing file with errors section",
			existingConfig: `errors:
  exclude_checks:
    - SVR00001
    - SVR00002
`,
			expectModified: true,
			wantErr:        false,
		},
		{
			name: "updates incorrect docs_structure_enforced",
			existingConfig: `docs_structure_enforced:
  enabled: false
  version: 1
`,
			expectModified: true,
			wantErr:        false,
		},
		{
			name: "no changes needed when already correct",
			existingConfig: `docs_structure_enforced:
  enabled: true
  version: 1
`,
			expectModified: false,
			wantErr:        false,
		},
		{
			name: "preserves errors section and adds docs_structure_enforced",
			existingConfig: `errors:
  exclude_checks:
    - SVR00001
`,
			expectModified: true,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tmpDir := t.TempDir()

			// Write existing config if provided
			validationPath := filepath.Join(tmpDir, "validation.yml")
			if tt.existingConfig != "" {
				if err := os.WriteFile(validationPath, []byte(tt.existingConfig), 0644); err != nil {
					t.Fatalf("Failed to write test config: %v", err)
				}
			}

			// Run the function with enabled=true and version=1
			desiredConfig := DocsStructureEnforcedConfig{
				Enabled: true,
				Version: 1,
			}
			modified, err := EnsureDocsStructureEnforced(tmpDir, desiredConfig)

			// Check error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("EnsureDocsStructureEnforced() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check modified expectation
			if modified != tt.expectModified {
				t.Errorf("EnsureDocsStructureEnforced() modified = %v, expectModified %v", modified, tt.expectModified)
			}

			// Verify the file exists and has correct content
			data, err := os.ReadFile(validationPath)
			if err != nil {
				t.Fatalf("Failed to read result file: %v", err)
			}

			var config ValidationConfig
			if err := yaml.Unmarshal(data, &config); err != nil {
				t.Fatalf("Failed to parse result: %v", err)
			}

			// Verify docs_structure_enforced is set correctly
			if config.DocsStructureEnforced == nil {
				t.Error("docs_structure_enforced is nil")
			} else {
				if !config.DocsStructureEnforced.Enabled {
					t.Error("docs_structure_enforced.enabled should be true")
				}
				if config.DocsStructureEnforced.Version != 1 {
					t.Errorf("docs_structure_enforced.version = %d, want 1", config.DocsStructureEnforced.Version)
				}
			}

			// Verify errors section is preserved if it existed
			if tt.existingConfig != "" && len(tt.existingConfig) > 0 {
				var originalConfig ValidationConfig
				if err := yaml.Unmarshal([]byte(tt.existingConfig), &originalConfig); err == nil {
					if originalConfig.Errors != nil && len(originalConfig.Errors.ExcludeChecks) > 0 {
						if config.Errors == nil {
							t.Error("errors section was not preserved")
						} else if len(config.Errors.ExcludeChecks) != len(originalConfig.Errors.ExcludeChecks) {
							t.Errorf("exclude_checks count = %d, want %d",
								len(config.Errors.ExcludeChecks),
								len(originalConfig.Errors.ExcludeChecks))
						}
					}
				}
			}
		})
	}
}

func TestEnsureDocsStructureEnforced_CustomValues(t *testing.T) {
	tests := []struct {
		name           string
		enabled        bool
		version        int
		existingConfig string
		expectModified bool
	}{
		{
			name:           "enforces custom version 2",
			enabled:        true,
			version:        2,
			existingConfig: "",
			expectModified: true,
		},
		{
			name:    "enforces disabled state",
			enabled: false,
			version: 1,
			existingConfig: `docs_structure_enforced:
  enabled: true
  version: 1
`,
			expectModified: true,
		},
		{
			name:    "no change when custom values already match",
			enabled: true,
			version: 3,
			existingConfig: `docs_structure_enforced:
  enabled: true
  version: 3
`,
			expectModified: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			validationPath := filepath.Join(tmpDir, "validation.yml")

			if tt.existingConfig != "" {
				if err := os.WriteFile(validationPath, []byte(tt.existingConfig), 0644); err != nil {
					t.Fatalf("Failed to write test config: %v", err)
				}
			}

			desiredConfig := DocsStructureEnforcedConfig{
				Enabled: tt.enabled,
				Version: tt.version,
			}
			modified, err := EnsureDocsStructureEnforced(tmpDir, desiredConfig)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if modified != tt.expectModified {
				t.Errorf("EnsureDocsStructureEnforced() modified = %v, expectModified %v", modified, tt.expectModified)
			}

			// Verify the file has the correct custom values
			data, err := os.ReadFile(validationPath)
			if err != nil {
				t.Fatalf("Failed to read result file: %v", err)
			}

			var config ValidationConfig
			if err := yaml.Unmarshal(data, &config); err != nil {
				t.Fatalf("Failed to parse result: %v", err)
			}

			if config.DocsStructureEnforced == nil {
				t.Fatal("docs_structure_enforced is nil")
			}

			if config.DocsStructureEnforced.Enabled != tt.enabled {
				t.Errorf("enabled = %v, want %v", config.DocsStructureEnforced.Enabled, tt.enabled)
			}

			if config.DocsStructureEnforced.Version != tt.version {
				t.Errorf("version = %v, want %v", config.DocsStructureEnforced.Version, tt.version)
			}
		})
	}
}

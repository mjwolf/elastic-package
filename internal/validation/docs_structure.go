// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ValidationConfig represents the structure of validation.yml
type ValidationConfig struct {
	Errors                *ErrorsConfig                `yaml:"errors,omitempty"`
	DocsStructureEnforced *DocsStructureEnforcedConfig `yaml:"docs_structure_enforced,omitempty"`
}

// ErrorsConfig represents the errors section in validation.yml
type ErrorsConfig struct {
	ExcludeChecks []string `yaml:"exclude_checks,omitempty"`
}

// DocsStructureEnforcedConfig represents the docs_structure_enforced section
type DocsStructureEnforcedConfig struct {
	Enabled bool `yaml:"enabled"`
	Version int  `yaml:"version"`
}

// EnsureDocsStructureEnforced ensures that validation.yml contains the required
// docs_structure_enforced configuration. It creates the file if it doesn't exist,
// or updates it if the configuration is missing or incorrect.
// The desired parameter specifies the desired configuration values.
// Returns true if the file was created or modified, false if no changes were needed.
func EnsureDocsStructureEnforced(packageRoot string, desired DocsStructureEnforcedConfig) (bool, error) {
	validationPath := filepath.Join(packageRoot, "validation.yml")

	// Check if file exists
	data, err := os.ReadFile(validationPath)
	fileExists := !errors.Is(err, os.ErrNotExist)
	if err != nil && fileExists {
		return false, fmt.Errorf("failed to read validation.yml: %w", err)
	}

	var config ValidationConfig

	// Parse existing file if it exists
	if fileExists {
		if err := yaml.Unmarshal(data, &config); err != nil {
			return false, fmt.Errorf("failed to parse validation.yml: %w", err)
		}
	}

	// Check if docs_structure_enforced is already correctly configured
	if config.DocsStructureEnforced != nil &&
		config.DocsStructureEnforced.Enabled == desired.Enabled &&
		config.DocsStructureEnforced.Version == desired.Version {
		return false, nil // Already configured correctly
	}

	// Set or update docs_structure_enforced
	config.DocsStructureEnforced = &DocsStructureEnforcedConfig{
		Enabled: desired.Enabled,
		Version: desired.Version,
	}

	// Marshal back to YAML
	output, err := yaml.Marshal(&config)
	if err != nil {
		return false, fmt.Errorf("failed to marshal validation config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(validationPath, output, 0644); err != nil {
		return false, fmt.Errorf("failed to write validation.yml: %w", err)
	}

	return true, nil
}

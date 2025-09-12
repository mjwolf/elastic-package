// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package llmagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlecAivazis/survey/v2"

	"github.com/elastic/elastic-package/internal/packages"
)

// DocumentationAgent handles documentation updates for packages
type DocumentationAgent struct {
	agent           *Agent
	packageRoot     string
	templateContent string
}

// NewDocumentationAgent creates a new documentation agent
func NewDocumentationAgent(provider LLMProvider, packageRoot string) (*DocumentationAgent, error) {
	// Read the template file
	templatePath := filepath.Join("internal", "packages", "archetype", "_static", "package-docs-readme.md.tmpl")
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}

	// Create tools for package operations
	tools := PackageTools(packageRoot)

	// Create the agent
	agent := NewAgent(provider, tools)

	return &DocumentationAgent{
		agent:           agent,
		packageRoot:     packageRoot,
		templateContent: string(templateContent),
	}, nil
}

// UpdateDocumentation runs the interactive documentation update process
func (d *DocumentationAgent) UpdateDocumentation(ctx context.Context) error {
	// Read package manifest for context
	manifest, err := packages.ReadPackageManifestFromPackageRoot(d.packageRoot)
	if err != nil {
		return fmt.Errorf("failed to read package manifest: %w", err)
	}

	// Create the initial prompt
	prompt := d.buildInitialPrompt(manifest)

	fmt.Println("Starting documentation update process...")
	fmt.Println("The LLM agent will analyze your package and update the documentation.")
	fmt.Println()

	// Interactive loop
	for {
		fmt.Println("🤖 LLM Agent is working...")

		// Execute the task
		result, err := d.agent.ExecuteTask(ctx, prompt)
		if err != nil {
			return fmt.Errorf("agent task failed: %w", err)
		}

		// Show the result
		fmt.Println("\n📝 Agent Response:")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Println(result.FinalContent)
		fmt.Println(strings.Repeat("-", 50))

		// Check if README.md was created/updated
		readmeExists := d.checkReadmeExists()
		if readmeExists {
			content, err := d.readCurrentReadme()
			if err == nil && content != "" {
				fmt.Println("\n📄 Current README.md preview:")
				fmt.Println(strings.Repeat("=", 50))
				// Show first 1000 characters
				if len(content) > 1000 {
					fmt.Println(content[:1000] + "...")
					fmt.Printf("\n(Showing first 1000 characters of %d total)\n", len(content))
				} else {
					fmt.Println(content)
				}
				fmt.Println(strings.Repeat("=", 50))
			}
		}

		// Ask user what to do next
		var action string
		err = survey.AskOne(&survey.Select{
			Message: "What would you like to do?",
			Options: []string{
				"Accept and finalize",
				"Request changes",
				"Cancel",
			},
			Default: "Accept and finalize",
		}, &action)

		if err != nil {
			return fmt.Errorf("prompt failed: %w", err)
		}

		switch action {
		case "Accept and finalize":
			if readmeExists {
				fmt.Println("✅ Documentation update completed!")
				return nil
			} else {
				fmt.Println("⚠️  No README.md was created. Continuing...")
				prompt = "You haven't created a README.md file yet. Please create the README.md file in the _dev_/docs/ directory based on your analysis."
			}

		case "Request changes":
			var changes string
			err = survey.AskOne(&survey.Multiline{
				Message: "What changes would you like to make?",
			}, &changes)

			if err != nil {
				return fmt.Errorf("prompt failed: %w", err)
			}

			prompt = fmt.Sprintf("Please make the following changes to the documentation:\n\n%s", changes)

		case "Cancel":
			fmt.Println("❌ Documentation update cancelled.")
			return nil
		}
	}
}

// UpdateDocumentationNonInteractive runs the documentation update process without user interaction
func (d *DocumentationAgent) UpdateDocumentationNonInteractive(ctx context.Context) error {
	// Read package manifest for context
	manifest, err := packages.ReadPackageManifestFromPackageRoot(d.packageRoot)
	if err != nil {
		return fmt.Errorf("failed to read package manifest: %w", err)
	}

	// Create the initial prompt
	prompt := d.buildInitialPrompt(manifest)

	fmt.Println("Starting non-interactive documentation update process...")
	fmt.Println("The LLM agent will analyze your package and generate documentation automatically.")
	fmt.Println()

	// Execute the task once
	fmt.Println("🤖 LLM Agent is working...")
	result, err := d.agent.ExecuteTask(ctx, prompt)
	if err != nil {
		return fmt.Errorf("agent task failed: %w", err)
	}

	// Show the result
	fmt.Println("\n📝 Agent Response:")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Println(result.FinalContent)
	fmt.Println(strings.Repeat("-", 50))

	// Check if README.md was created/updated
	readmeExists := d.checkReadmeExists()
	if readmeExists {
		content, err := d.readCurrentReadme()
		if err == nil && content != "" {
			fmt.Println("\n📄 README.md was created successfully!")
			fmt.Printf("✅ Documentation update completed! (%d characters written)\n", len(content))
			return nil
		}
	}

	// If no README was created, try once more with a specific prompt
	fmt.Println("⚠️  No README.md was created. Trying again with specific instructions...")
	specificPrompt := "You haven't created a README.md file yet. Please create the README.md file in the _dev_/docs/ directory based on your analysis. This is required to complete the task."

	_, err = d.agent.ExecuteTask(ctx, specificPrompt)
	if err != nil {
		return fmt.Errorf("second attempt failed: %w", err)
	}

	// Check again
	readmeExists = d.checkReadmeExists()
	if readmeExists {
		content, err := d.readCurrentReadme()
		if err == nil && content != "" {
			fmt.Println("\n📄 README.md was created on second attempt!")
			fmt.Printf("✅ Documentation update completed! (%d characters written)\n", len(content))
			return nil
		}
	}

	return fmt.Errorf("failed to create README.md after two attempts")
}

// buildInitialPrompt creates the initial prompt for the LLM
func (d *DocumentationAgent) buildInitialPrompt(manifest *packages.PackageManifest) string {
	return fmt.Sprintf(`You are a documentation assistant for Elastic Integrations. Your task is to analyze the current package and update/create the README.md file in the _dev_/docs/ directory.

Package Information:
- Name: %s
- Title: %s  
- Type: %s
- Version: %s
- Description: %s

You have access to the following tools:
- list_directory: List files and directories in the package
- read_file: Read the contents of any file in the package
- write_file: Write content to any file in the package

Template to follow:
The README.md should be based on this template:

%s

Your tasks:
1. First, explore the package structure to understand what it contains
2. Read existing documentation if any exists
3. Analyze data streams, manifests, fields, and other relevant files
4. Create or update the _dev_/docs/README.md file following the template structure
5. Fill in all the placeholder sections with relevant information from the package
6. Ensure the documentation is comprehensive and helpful for users
7. If you are unsure 

Please start by exploring the package structure to understand what you're working with.`,
		manifest.Name,
		manifest.Title,
		manifest.Type,
		manifest.Version,
		manifest.Description,
		d.templateContent)
}

// checkReadmeExists checks if README.md exists in _dev_/docs/
func (d *DocumentationAgent) checkReadmeExists() bool {
	readmePath := filepath.Join(d.packageRoot, "_dev_", "docs", "README.md")
	_, err := os.Stat(readmePath)
	return err == nil
}

// readCurrentReadme reads the current README.md content
func (d *DocumentationAgent) readCurrentReadme() (string, error) {
	readmePath := filepath.Join(d.packageRoot, "_dev_", "docs", "README.md")
	content, err := os.ReadFile(readmePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

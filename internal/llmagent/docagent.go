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

	"github.com/elastic/elastic-package/internal/logger"
	"github.com/elastic/elastic-package/internal/packages"
	"github.com/elastic/elastic-package/internal/packages/archetype"
)

// DocumentationAgent handles documentation updates for packages
type DocumentationAgent struct {
	agent           *Agent
	packageRoot     string
	templateContent string
}

// NewDocumentationAgent creates a new documentation agent
func NewDocumentationAgent(provider LLMProvider, packageRoot string) (*DocumentationAgent, error) {
	// Get the embedded template content
	templateContent := archetype.GetPackageDocsReadmeTemplate()

	// Create tools for package operations
	tools := PackageTools(packageRoot)

	// Create the agent
	agent := NewAgent(provider, tools)

	return &DocumentationAgent{
		agent:           agent,
		packageRoot:     packageRoot,
		templateContent: templateContent,
	}, nil
}

// UpdateDocumentation runs the documentation update process
func (d *DocumentationAgent) UpdateDocumentation(ctx context.Context, nonInteractive bool) error {
	// Read package manifest for context
	manifest, err := packages.ReadPackageManifestFromPackageRoot(d.packageRoot)
	if err != nil {
		return fmt.Errorf("failed to read package manifest: %w", err)
	}

	// Create the initial prompt
	prompt := d.buildInitialPrompt(manifest)

	if nonInteractive {
		fmt.Println("Starting non-interactive documentation update process...")
		fmt.Println("The LLM agent will analyze your package and generate documentation automatically.")
		fmt.Println()

		// Execute the task once
		fmt.Println("🤖 LLM Agent is working...")
		result, err := d.agent.ExecuteTask(ctx, prompt)
		if err != nil {
			return fmt.Errorf("agent task failed: %w", err)
		}

		// Debug logging for the full agent task response
		logger.Debugf("DEBUG: Full agent task response follows (may contain sensitive content)")
		logger.Debugf("Agent task response - Success: %t", result.Success)
		logger.Debugf("Agent task response - FinalContent: %s", result.FinalContent)
		logger.Debugf("Agent task response - Conversation entries: %d", len(result.Conversation))
		for i, entry := range result.Conversation {
			logger.Debugf("Agent task response - Conversation[%d]: type=%s, content_length=%d",
				i, entry.Type, len(entry.Content))
			logger.Tracef("Agent task response - Conversation[%d]: content=%s", i, entry.Content)
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
		specificPrompt := "You haven't created a README.md file yet. Please create the README.md file in the _dev/build/docs/ directory based on your analysis. This is required to complete the task."

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

	// Interactive mode
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

		// Debug logging for the full agent task response
		logger.Debugf("DEBUG: Full agent task response follows (may contain sensitive content)")
		logger.Debugf("Agent task response - Success: %t", result.Success)
		logger.Debugf("Agent task response - FinalContent: %s", result.FinalContent)
		logger.Debugf("Agent task response - Conversation entries: %d", len(result.Conversation))
		for i, entry := range result.Conversation {
			logger.Debugf("Agent task response - Conversation[%d]: type=%s, content_length=%d",
				i, entry.Type, len(entry.Content))
			logger.Tracef("Agent task response - Conversation[%d]: content=%s", i, entry.Content)
		}

		// Show the result
		fmt.Println("\n📝 Agent Response:")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Println(result.FinalContent)
		fmt.Println(strings.Repeat("-", 50))

		// Check if README.md was created/updated and always show full content before prompting
		readmeExists := d.checkReadmeExists()
		if readmeExists {
			content, err := d.readCurrentReadme()
			if err == nil && content != "" {
				fmt.Println("\n📄 Current README.md content (_dev/build/docs/README.md):")
				fmt.Println(strings.Repeat("=", 70))
				fmt.Println(content)
				fmt.Println(strings.Repeat("=", 70))
				fmt.Printf("📊 File stats: %d characters, %d lines\n", len(content), strings.Count(content, "\n")+1)
			} else {
				fmt.Println("\n⚠️  README.md file exists but could not be read or is empty")
			}
		} else {
			fmt.Println("\n⚠️  No README.md file found at _dev/build/docs/README.md")
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
				// Ask user if they want to continue or exit anyway
				var continueChoice string
				err = survey.AskOne(&survey.Select{
					Message: "No README.md file was created. What would you like to do?",
					Options: []string{
						"Try again",
						"Exit anyway",
					},
					Default: "Try again",
				}, &continueChoice)

				if err != nil {
					return fmt.Errorf("prompt failed: %w", err)
				}

				if continueChoice == "Exit anyway" {
					fmt.Println("⚠️  Exiting without creating README.md file.")
					return nil
				}

				fmt.Println("🔄 Trying again to create README.md...")
				prompt = "You haven't created a README.md file yet. Please create the README.md file in the _dev/build/docs/ directory based on your analysis."
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

// buildInitialPrompt creates the initial prompt for the LLM
func (d *DocumentationAgent) buildInitialPrompt(manifest *packages.PackageManifest) string {
	return fmt.Sprintf(`You are a documentation assistant for Elastic Integrations. Your task is to analyze the current package and update/create the README.md file in the _dev/build/docs/ directory.

IMPORTANT FILE RESTRICTIONS:
- ONLY work with "_dev/build/docs/README.md" - this is the source documentation file
- NEVER read or write "_docs/README.md" - this is a generated artifact and should not be modified
- The "_docs/" directory contains generated files that are created during the build process

Package Information:
- Name: %s
- Title: %s  
- Type: %s
- Version: %s
- Description: %s

You have access to the following tools:
- list_directory: List files and directories in the package
- read_file: Read the contents of any file in the package (except generated artifacts)
- write_file: Write content to files in the package (only to source files, not generated artifacts)

Template to follow:
The README.md should be based on this template:

%s

Your tasks:
1. First, explore the package structure to understand what it contains
2. Read existing documentation if any exists (from _dev/build/docs/README.md only)
3. Analyze data streams, manifests, fields, and other relevant files
4. Create or update the _dev/build/docs/README.md file following the template structure
5. Fill in all the placeholder sections with relevant information from the package
6. Ensure the documentation is comprehensive and helpful for users

Remember: Always work with "_dev/build/docs/README.md" as the source file. Never touch "_docs/README.md" or any files in the "_docs/" directory.

Please start by exploring the package structure to understand what you're working with.`,
		manifest.Name,
		manifest.Title,
		manifest.Type,
		manifest.Version,
		manifest.Description,
		d.templateContent)
}

// checkReadmeExists checks if README.md exists in _dev/build/docs/
func (d *DocumentationAgent) checkReadmeExists() bool {
	readmePath := filepath.Join(d.packageRoot, "_dev", "build", "docs", "README.md")
	_, err := os.Stat(readmePath)
	return err == nil
}

// readCurrentReadme reads the current README.md content
func (d *DocumentationAgent) readCurrentReadme() (string, error) {
	readmePath := filepath.Join(d.packageRoot, "_dev", "build", "docs", "README.md")
	content, err := os.ReadFile(readmePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

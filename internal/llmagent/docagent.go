// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package llmagent

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elastic/elastic-package/internal/docs"
	"github.com/elastic/elastic-package/internal/logger"
	"github.com/elastic/elastic-package/internal/packages"
	"github.com/elastic/elastic-package/internal/packages/archetype"
	"github.com/elastic/elastic-package/internal/tui"
)

// The embedded example_readme is an example of a high-quality integration readme, following the static template archetype,
// That helps the LLM follow an example.
//
//go:embed _static/example_readme.md
var exampleReadmeContent string

// DocumentationAgent handles documentation updates for packages
type DocumentationAgent struct {
	agent                 *Agent
	packageRoot           string
	templateContent       string
	originalReadmeContent *string // Stores original README content for restoration on cancel
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

	// Backup original README content before making any changes
	d.backupOriginalReadme()

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
			fmt.Println("❌ Agent task failed")
			return fmt.Errorf("agent task failed: %w", err)
		} else {
			fmt.Println("✅ Task completed")
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

		// Check if the response indicates an error occurred
		if isErrorResponse(result.FinalContent) {
			fmt.Println("\n❌ Error detected in LLM response.")
			fmt.Println("In non-interactive mode, exiting due to error.")
			return fmt.Errorf("LLM agent encountered an error: %s", result.FinalContent)
		}

		// Check if README.md was updated
		readmeUpdated := d.checkReadmeUpdated()
		if readmeUpdated {
			content, err := d.readCurrentReadme()
			if err == nil && content != "" {
				fmt.Println("\n📄 README.md was updated successfully!")
				fmt.Printf("✅ Documentation update completed! (%d characters written)\n", len(content))
				return nil
			}
		}

		// If no README was updated, try once more with a specific prompt
		fmt.Println("⚠️  No README.md was updated. Trying again with specific instructions...")
		specificPrompt := "You haven't updated a README.md file yet. Please create the README.md file in the _dev/build/docs/ directory based on your analysis. This is required to complete the task."

		_, err = d.agent.ExecuteTask(ctx, specificPrompt)
		if err != nil {
			return fmt.Errorf("second attempt failed: %w", err)
		}

		// Check again
		readmeUpdated = d.checkReadmeUpdated()
		if readmeUpdated {
			content, err := d.readCurrentReadme()
			if err == nil && content != "" {
				fmt.Println("\n📄 README.md was updated on second attempt!")
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
			fmt.Println("❌ Agent task failed")
			return fmt.Errorf("agent task failed: %w", err)
		} else {
			fmt.Println("✅ Task completed")
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

		// Check if the response indicates an error occurred
		if isErrorResponse(result.FinalContent) {
			fmt.Println("\n❌ Error detected in LLM response.")

			// Ask user what to do about the error
			errorPrompt := tui.NewSelect("What would you like to do?", []string{
				"Try again",
				"Exit",
			}, "Try again")

			var errorAction string
			err = tui.AskOne(errorPrompt, &errorAction)
			if err != nil {
				return fmt.Errorf("prompt failed: %w", err)
			}

			if errorAction == "Exit" {
				fmt.Println("⚠️  Exiting due to LLM error.")
				d.restoreOriginalReadme()
				return fmt.Errorf("user chose to exit due to LLM error")
			}

			// Continue the loop to try again with comprehensive context
			prompt = d.buildRevisionPrompt("The previous attempt encountered an error. Please try a different approach to analyze the package and create/update the documentation.")
			continue
		}

		// Check if README.md was updated and show processed content in scrollable viewer
		readmeUpdated := d.checkReadmeUpdated()
		if readmeUpdated {
			sourceContent, err := d.readCurrentReadme()
			if err == nil && sourceContent != "" {
				// Generate the processed README using the same logic as elastic-package build
				renderedContent, shouldBeRendered, err := docs.GenerateReadme("README.md", d.packageRoot)
				if err != nil || !shouldBeRendered {
					fmt.Println("\n⚠️  The generated README.md could not be rendered.")
					fmt.Println("It's recommended that you do not accept this version (ask for revisions or cancel).")
				} else {
					// Show the processed/rendered content
					processedContentStr := string(renderedContent)
					fmt.Printf("📊 Processed README stats: %d characters, %d lines\n", len(processedContentStr), strings.Count(processedContentStr, "\n")+1)

					title := "📄 Processed README.md (as generated by elastic-package build)"
					if err := tui.ShowContent(title, processedContentStr); err != nil {
						// Fallback to simple print if viewer fails
						fmt.Printf("\n%s:\n", title)
						fmt.Println(strings.Repeat("=", 70))
						fmt.Println(processedContentStr)
						fmt.Println(strings.Repeat("=", 70))
					}
				}
			} else {
				fmt.Println("\n⚠️  README.md file exists but could not be read or is empty")
			}
		} else {
			fmt.Println("\n⚠️  README.md file not updated")
		}

		// Ask user what to do next
		selectPrompt := tui.NewSelect("What would you like to do?", []string{
			"Accept and finalize",
			"Request changes",
			"Cancel",
		}, "Accept and finalize")

		var action string
		err = tui.AskOne(selectPrompt, &action)
		if err != nil {
			return fmt.Errorf("prompt failed: %w", err)
		}

		switch action {
		case "Accept and finalize":
			if readmeUpdated {
				// Validate that human-edited sections were preserved if we had original content
				if d.originalReadmeContent != nil {
					if newContent, err := d.readCurrentReadme(); err == nil {
						warnings := d.validatePreservedSections(*d.originalReadmeContent, newContent)
						if len(warnings) > 0 {
							fmt.Println("⚠️  Warning: Some human-edited sections may not have been preserved:")
							for _, warning := range warnings {
								fmt.Printf("   - %s\n", warning)
							}
							fmt.Println("   Please review the documentation to ensure important content wasn't lost.")
						}
					}
				}

				fmt.Println("✅ Documentation update completed!")
				return nil
			}

			// No content found in response and no file exists
			// Ask user if they want to continue or exit anyway
			continuePrompt := tui.NewSelect("README.md file wasn't updated. What would you like to do?", []string{
				"Try again",
				"Exit anyway",
			}, "Try again")

			var continueChoice string
			err = tui.AskOne(continuePrompt, &continueChoice)
			if err != nil {
				return fmt.Errorf("prompt failed: %w", err)
			}

			if continueChoice == "Exit anyway" {
				fmt.Println("⚠️  Exiting without creating README.md file.")
				d.restoreOriginalReadme()
				return nil
			}

			fmt.Println("🔄 Trying again to create README.md...")
			prompt = d.buildRevisionPrompt("You haven't written a README.md file yet. Please write the README.md file in the _dev/build/docs/ directory based on your analysis.")

		case "Request changes":
			changes, err := tui.AskTextArea("What changes would you like to make to the documentation?")
			if err != nil {
				// Check if user cancelled (pressed ESC)
				if errors.Is(err, tui.ErrCancelled) {
					fmt.Println("⚠️  Changes request cancelled.")
					continue // Go back to the main menu
				}
				return fmt.Errorf("prompt failed: %w", err)
			}

			// Check if no changes were provided
			if strings.TrimSpace(changes) == "" {
				fmt.Println("⚠️  No changes specified. Please try again.")
				continue
			}

			prompt = d.buildRevisionPrompt(changes)

		case "Cancel":
			fmt.Println("❌ Documentation update cancelled.")
			d.restoreOriginalReadme()
			return nil
		}
	}
}

// buildInitialPrompt creates the initial prompt for the LLM
func (d *DocumentationAgent) buildInitialPrompt(manifest *packages.PackageManifest) string {
	return fmt.Sprintf(`You are an expert technical writer specializing in documentation for Elastic Integrations. Your mission is to create a comprehensive, user-friendly README.md file by synthesizing information from the integration's source code, external research, and a provided template.

Core Task:

Generate or update the _dev/build/docs/README.md file for the integration specified below.

* Package Name: %s
* Title: %s
* Type: %s
* Version: %s
* Description: %s


Critical Directives (Follow These Strictly):

1.  File Restriction: You MUST ONLY write to the _dev/build/docs/README.md file. Do not modify any other files.
2.  Preserve Human Content: You MUST preserve any content between <!-- HUMAN-EDITED START --> and <!-- HUMAN-EDITED END --> comment blocks. This content is non-negotiable and must be kept verbatim in its original position.
3.  No Hallucination: If you cannot find a piece of information in the package files or through web search, DO NOT invent it. Instead, insert a clear placeholder in the document: << INFORMATION NOT AVAILABLE - PLEASE UPDATE >>.

Your Step-by-Step Process:

1.  Initial Analysis:
    * Begin by listing the contents of the package to understand its structure.
    * Read the existing _dev/build/docs/README.md (if it exists) to identify its current state and locate any human-edited sections that must be preserved.

2.  Internal Information Gathering:
    * Analyze the package files to extract key details. Pay close attention to:
        * manifest.yml: For top-level metadata, owner, license, and supported Elasticsearch versions.
        * data_stream/*/manifest.yml: To compile a list of all data streams, their types (logs, metrics), and a brief description of the data each collects.
        * data_stream/*/fields/fields.yml: To understand the data schema and important fields. Mentioning a few key fields can be helpful for users.

3.  External Information Gathering:
    * Use your web search tool to find the official documentation for the service or technology this integration supports (e.g., "NGINX logs setup," "AWS S3 access logs format").
    * Your goal is to find **actionable, step-by-step instructions** for users on how to configure the *source system* to generate the data this integration is designed to collect.

4.  Drafting the Documentation:
    * Using the provided template, begin writing the README.md.
    * Integrate the information gathered from the package files and your web research into the appropriate sections.
    * Re-insert any preserved human-edited sections into their original locations.

5.  Review and Finalize:
    * Read through your generated README to ensure it is clear, accurate, and easy to follow.
    * Verify that all critical directives (file restrictions, content preservation) have been followed.
    * Confirm that the tone and style align with the provided high-quality example.

6. Write the results:
    * Write the generated README to _dev/build/docs/README.md.
    * Do not return the results as a response in this conversation.

Style and Content Guidance:

* Audience & Tone: Write for a technical audience (e.g., DevOps Engineers, SREs, Security Analysts). The tone should be professional, clear, and direct. Use active voice.
* Template is a Blueprint: The provided template is your required structure. Follow it closely.
* The Example is Your "Gold Standard": The provided example README demonstrates the target quality, level of detail, and formatting. Emulate its style, especially in the "Configuration" and "Setup" sections. Explain *why* a step is needed, not just *what* the step is.
* Be Specific: Instead of saying "configure the service," provide a concrete configuration snippet or a numbered list of steps. Link to official external documentation where appropriate to provide users with more depth.

Assets:

* Template to Follow:
    %s

* Example of a High-Quality README:
    %s

Please begin. Start with the "Initial Analysis" step.`,
		manifest.Name,
		manifest.Title,
		manifest.Type,
		manifest.Version,
		manifest.Description,
		d.templateContent,
		exampleReadmeContent)
}

// checkReadmeUpdated checks if README.md has been updated by comparing current content to originalReadmeContent
func (d *DocumentationAgent) checkReadmeUpdated() bool {
	readmePath := filepath.Join(d.packageRoot, "_dev", "build", "docs", "README.md")

	// Check if file exists
	if _, err := os.Stat(readmePath); err != nil {
		return false
	}

	// Read current content
	currentContent, err := os.ReadFile(readmePath)
	if err != nil {
		return false
	}

	currentContentStr := string(currentContent)

	// If there was no original content, any new content means it's updated
	if d.originalReadmeContent == nil {
		return currentContentStr != ""
	}

	// Compare current content with original content
	return currentContentStr != *d.originalReadmeContent
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

// validatePreservedSections checks if human-edited sections are preserved in the new content
func (d *DocumentationAgent) validatePreservedSections(originalContent, newContent string) []string {
	var warnings []string

	// Extract preserved sections from original content
	preservedSections := d.extractPreservedSections(originalContent)

	// Check if each preserved section exists in the new content
	for marker, content := range preservedSections {
		if !strings.Contains(newContent, content) {
			warnings = append(warnings, fmt.Sprintf("Human-edited section '%s' was not preserved", marker))
		}
	}

	return warnings
}

// isErrorResponse detects if the LLM response indicates an error occurred
func isErrorResponse(content string) bool {
	errorIndicators := []string{
		"I encountered an error",
		"I'm experiencing an error",
		"I cannot complete",
		"I'm unable to complete",
		"Something went wrong",
		"There was an error",
		"I'm having trouble",
		"I failed to",
		"Error occurred",
		"Task did not complete within maximum iterations",
	}

	contentLower := strings.ToLower(content)
	for _, indicator := range errorIndicators {
		if strings.Contains(contentLower, strings.ToLower(indicator)) {
			return true
		}
	}
	return false
}

// extractPreservedSections extracts all human-edited sections from content
func (d *DocumentationAgent) extractPreservedSections(content string) map[string]string {
	sections := make(map[string]string)

	// Define marker pairs
	markers := []struct {
		start, end string
		name       string
	}{
		{"<!-- HUMAN-EDITED START -->", "<!-- HUMAN-EDITED END -->", "HUMAN-EDITED"},
		{"<!-- PRESERVE START -->", "<!-- PRESERVE END -->", "PRESERVE"},
	}

	for _, marker := range markers {
		startIdx := 0
		sectionNum := 1

		for {
			start := strings.Index(content[startIdx:], marker.start)
			if start == -1 {
				break
			}
			start += startIdx

			end := strings.Index(content[start:], marker.end)
			if end == -1 {
				break
			}
			end += start

			// Extract the full section including markers
			sectionContent := content[start : end+len(marker.end)]
			sectionKey := fmt.Sprintf("%s-%d", marker.name, sectionNum)
			sections[sectionKey] = sectionContent

			startIdx = end + len(marker.end)
			sectionNum++
		}
	}

	return sections
}

// buildRevisionPrompt creates a comprehensive prompt for document revisions that includes all necessary context
func (d *DocumentationAgent) buildRevisionPrompt(changes string) string {
	// Read package manifest for context
	manifest, err := packages.ReadPackageManifestFromPackageRoot(d.packageRoot)
	if err != nil {
		// Fallback to a simpler prompt if we can't read the manifest
		return fmt.Sprintf("Please make the following changes to the documentation:\n\n%s", changes)
	}

	return fmt.Sprintf(`You are continuing to work on documentation for an Elastic Integration. You have access to tools to analyze the package and make changes.

CURRENT TASK: Make specific revisions to the existing documentation based on user feedback.

Package Information:
* Package Name: %s
* Title: %s
* Type: %s
* Version: %s
* Description: %s

Critical Directives (Follow These Strictly):
1. File Restriction: You MUST ONLY write to the _dev/build/docs/README.md file. Do not modify any other files.
2. Preserve Human Content: You MUST preserve any content between <!-- HUMAN-EDITED START --> and <!-- HUMAN-EDITED END --> comment blocks.
3. Read Current Content: First read the existing _dev/build/docs/README.md to understand the current state.
4. No Hallucination: If you need information not available in package files, insert placeholders: << INFORMATION NOT AVAILABLE - PLEASE UPDATE >>.

Your Step-by-Step Process:
1. Read the current _dev/build/docs/README.md file to understand what exists
2. Analyze the requested changes carefully
3. Use available tools to gather any additional information needed
4. Make the specific changes requested while preserving existing good content
5. Ensure the result is comprehensive and follows Elastic documentation standards
6. Write the generated README to _dev/build/docs/README.md

User-Requested Changes:
%s

Template Reference:
%s

High-Quality Example:
%s

Begin by reading the current README.md file, then implement the requested changes thoughtfully.`,
		manifest.Name,
		manifest.Title,
		manifest.Type,
		manifest.Version,
		manifest.Description,
		changes,
		d.templateContent,
		exampleReadmeContent)
}

// backupOriginalReadme stores the current README content for potential restoration
func (d *DocumentationAgent) backupOriginalReadme() {
	readmePath := filepath.Join(d.packageRoot, "_dev", "build", "docs", "README.md")

	// Check if README exists
	if _, err := os.Stat(readmePath); err == nil {
		// Read and store the original content
		if content, err := os.ReadFile(readmePath); err == nil {
			contentStr := string(content)
			d.originalReadmeContent = &contentStr
			fmt.Printf("📋 Backed up original README.md (%d characters)\n", len(contentStr))
		} else {
			fmt.Printf("⚠️  Could not read original README.md for backup: %v\n", err)
		}
	} else {
		d.originalReadmeContent = nil
		fmt.Println("📋 No existing README.md found - will create new one")
	}
}

// restoreOriginalReadme restores the README to its original state
func (d *DocumentationAgent) restoreOriginalReadme() {
	readmePath := filepath.Join(d.packageRoot, "_dev", "build", "docs", "README.md")

	if d.originalReadmeContent != nil {
		// Restore original content
		if err := os.WriteFile(readmePath, []byte(*d.originalReadmeContent), 0o644); err != nil {
			fmt.Printf("⚠️  Failed to restore original README.md: %v\n", err)
		} else {
			fmt.Printf("🔄 Restored original README.md (%d characters)\n", len(*d.originalReadmeContent))
		}
	} else {
		// No original file existed, so remove any file that was created
		if err := os.Remove(readmePath); err != nil {
			if !os.IsNotExist(err) {
				fmt.Printf("⚠️  Failed to remove created README.md: %v\n", err)
			}
		} else {
			fmt.Println("🗑️  Removed created README.md file - restored to original state (no file)")
		}
	}
}

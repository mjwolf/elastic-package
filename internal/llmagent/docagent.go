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

//go:embed _static/example_readme.md
var exampleReadmeContent string

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

		result, err := d.agent.ExecuteTaskWithAnimation(ctx, prompt, nil)

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

	// Store original content for validation if it exists
	var originalReadmeContent string
	if d.checkReadmeExists() {
		if content, err := d.readCurrentReadme(); err == nil {
			originalReadmeContent = content
		}
	}

	// Interactive loop
	for {
		fmt.Println("🤖 LLM Agent is working...")

		// Execute the task
		result, err := d.agent.ExecuteTaskWithAnimation(ctx, prompt, nil)

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
				return fmt.Errorf("user chose to exit due to LLM error")
			}

			// Continue the loop to try again
			prompt = "The previous attempt encountered an error. Please try a different approach to analyze the package and create/update the documentation."
			continue
		}

		// Check if README.md was created/updated and show processed content in scrollable viewer
		readmeExists := d.checkReadmeExists()
		if readmeExists {
			sourceContent, err := d.readCurrentReadme()
			if err == nil && sourceContent != "" {
				fmt.Printf("\n📊 README.md source stats: %d characters, %d lines\n", len(sourceContent), strings.Count(sourceContent, "\n")+1)
				fmt.Println("📄 Press any key to view the processed README content (as it would appear after elastic-package build)...")

				// Generate the processed README using the same logic as elastic-package build
				renderedContent, shouldBeRendered, err := docs.GenerateReadmePreview("README.md", d.packageRoot)
				if err != nil {
					fmt.Printf("⚠️  Failed to generate processed README: %v\n", err)
					fmt.Println("📄 Showing source content instead:")

					// Fallback to source content
					title := "📄 Source README.md (_dev/build/docs/README.md)"
					if err := tui.ShowContent(title, sourceContent); err != nil {
						fmt.Println("\n📄 Source README.md content (_dev/build/docs/README.md):")
						fmt.Println(strings.Repeat("=", 70))
						fmt.Println(sourceContent)
						fmt.Println(strings.Repeat("=", 70))
					}
				} else if !shouldBeRendered {
					// Static file - show source content
					title := "📄 README.md (static file - no template processing)"
					if err := tui.ShowContent(title, sourceContent); err != nil {
						fmt.Println("\n📄 README.md content (static file):")
						fmt.Println(strings.Repeat("=", 70))
						fmt.Println(sourceContent)
						fmt.Println(strings.Repeat("=", 70))
					}
				} else {
					// Show the processed/rendered content
					processedContentStr := string(renderedContent)
					fmt.Printf("📊 Processed README stats: %d characters, %d lines\n", len(processedContentStr), strings.Count(processedContentStr, "\n")+1)

					title := "📄 Processed README.md (as generated by elastic-package build)"
					if err := tui.ShowContent(title, processedContentStr); err != nil {
						// Fallback to simple print if viewer fails
						fmt.Println("\n📄 Processed README.md content (as generated by elastic-package build):")
						fmt.Println(strings.Repeat("=", 70))
						fmt.Println(processedContentStr)
						fmt.Println(strings.Repeat("=", 70))
					}
				}
			} else {
				fmt.Println("\n⚠️  README.md file exists but could not be read or is empty")
			}
		} else {
			fmt.Println("\n⚠️  No README.md file found at _dev/build/docs/README.md")
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
			// Always try to write content from LLM response first, then check if file exists
			if d.tryWriteReadmeFromResponse(result) {
				// Validate preserved sections if we had original content
				if originalReadmeContent != "" {
					if newContent, err := d.readCurrentReadme(); err == nil {
						warnings := d.validatePreservedSections(originalReadmeContent, newContent)
						if len(warnings) > 0 {
							fmt.Println("⚠️  Warning: Some human-edited sections may not have been preserved:")
							for _, warning := range warnings {
								fmt.Printf("   - %s\n", warning)
							}
							fmt.Println("   Please review the documentation to ensure important content wasn't lost.")
						}
					}
				}
				fmt.Println("✅ Documentation created from LLM response and saved!")
				return nil
			}

			// Check if README was already created by the LLM using tools
			if readmeExists {
				// Validate that human-edited sections were preserved if we had original content
				if originalReadmeContent != "" {
					if newContent, err := d.readCurrentReadme(); err == nil {
						warnings := d.validatePreservedSections(originalReadmeContent, newContent)
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
			continuePrompt := tui.NewSelect("No README.md file was created. What would you like to do?", []string{
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
				return nil
			}

			fmt.Println("🔄 Trying again to create README.md...")
			prompt = "You haven't created a README.md file yet. Please create the README.md file in the _dev/build/docs/ directory based on your analysis."

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

			prompt = fmt.Sprintf("Please make the following changes to the documentation:\n\n%s", changes)

		case "Cancel":
			fmt.Println("❌ Documentation update cancelled.")
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
        * _dev/build/docs/configuration.yml: For default configuration options, variable names, and descriptions.

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

// tryWriteReadmeFromResponse attempts to extract README content from the LLM response and write it to disk
func (d *DocumentationAgent) tryWriteReadmeFromResponse(result *TaskResult) bool {
	if result == nil || result.FinalContent == "" {
		return false
	}

	// Look for markdown content patterns in the response
	content := result.FinalContent

	// Check if the response contains what looks like README content
	// Look for markdown headers, typical README sections, etc.
	if strings.Contains(content, "# ") ||
		strings.Contains(content, "## ") ||
		strings.Contains(content, "### ") ||
		(strings.Contains(content, "Integration") && strings.Contains(content, "Elastic")) {

		// Clean up the content - remove any assistant commentary
		readmeContent := d.extractReadmeContent(content)

		if readmeContent != "" {
			// Write to the README file
			readmePath := filepath.Join(d.packageRoot, "_dev", "build", "docs", "README.md")

			// Create directory if it doesn't exist
			dir := filepath.Dir(readmePath)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				fmt.Printf("⚠️  Failed to create directory: %v\n", err)
				return false
			}

			// Write the file
			if err := os.WriteFile(readmePath, []byte(readmeContent), 0o644); err != nil {
				fmt.Printf("⚠️  Failed to write README file: %v\n", err)
				return false
			}

			fmt.Printf("📝 Extracted and saved README content (%d characters)\n", len(readmeContent))
			return true
		}
	}

	return false
}

// extractReadmeContent cleans up and extracts the actual README content from LLM response
func (d *DocumentationAgent) extractReadmeContent(content string) string {
	lines := strings.Split(content, "\n")
	var readmeLines []string
	inCodeBlock := false
	foundReadmeStart := false

	for _, line := range lines {
		// Skip lines that look like assistant commentary
		if strings.HasPrefix(line, "I ") ||
			strings.HasPrefix(line, "I'll ") ||
			strings.HasPrefix(line, "Here's ") ||
			strings.HasPrefix(line, "Based on ") {
			continue
		}

		// Handle code blocks
		if strings.HasPrefix(line, "```") {
			inCodeBlock = !inCodeBlock
			if strings.Contains(line, "markdown") {
				foundReadmeStart = true
				continue
			}
			if foundReadmeStart && !inCodeBlock {
				// End of markdown code block - we got our content
				break
			}
			continue
		}

		// If we're in a markdown code block, collect the content
		if inCodeBlock && foundReadmeStart {
			readmeLines = append(readmeLines, line)
			continue
		}

		// If not in code block, look for direct markdown content
		if !inCodeBlock && (strings.HasPrefix(line, "# ") || foundReadmeStart) {
			foundReadmeStart = true
			readmeLines = append(readmeLines, line)
		}
	}

	// If no markdown code block was found, try to extract direct content
	if len(readmeLines) == 0 {
		foundReadmeStart = false
		for _, line := range lines {
			// Look for the start of README content
			if strings.HasPrefix(line, "# ") &&
				(strings.Contains(line, "Integration") || strings.Contains(line, "Package")) {
				foundReadmeStart = true
			}

			if foundReadmeStart {
				// Skip obvious assistant commentary
				if strings.HasPrefix(line, "I ") ||
					strings.HasPrefix(line, "This ") ||
					strings.HasPrefix(line, "Based on ") ||
					strings.HasPrefix(line, "Here's ") {
					continue
				}
				readmeLines = append(readmeLines, line)
			}
		}
	}

	result := strings.Join(readmeLines, "\n")
	result = strings.TrimSpace(result)

	// Ensure it looks like valid README content
	if len(result) > 100 && (strings.Contains(result, "# ") || strings.Contains(result, "## ")) {
		return result
	}

	return ""
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
		"Failed to",
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

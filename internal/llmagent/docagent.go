// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package llmagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	_ "embed"

	"github.com/elastic/elastic-package/internal/docs"
	"github.com/elastic/elastic-package/internal/logger"
	"github.com/elastic/elastic-package/internal/packages"
	"github.com/elastic/elastic-package/internal/tui"
)

//go:embed _static/initial_prompt.txt
var initialPrompt string

//go:embed _static/revision_prompt.txt
var revisionPrompt string

//go:embed _static/limit_hit_prompt.txt
var limitHitPrompt string

// DocumentationAgent handles documentation updates for packages
type DocumentationAgent struct {
	agent                 *Agent
	packageRoot           string
	originalReadmeContent *string // Stores original README content for restoration on cancel
}

// NewDocumentationAgent creates a new documentation agent
func NewDocumentationAgent(provider LLMProvider, packageRoot string) (*DocumentationAgent, error) {
	// Create tools for package operations
	tools := PackageTools(packageRoot)

	// Create the agent
	agent := NewAgent(provider, tools)

	return &DocumentationAgent{
		agent:       agent,
		packageRoot: packageRoot,
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
		return d.runNonInteractiveMode(ctx, prompt)
	}

	return d.runInteractiveMode(ctx, prompt)
}

// runNonInteractiveMode handles the non-interactive documentation update flow
func (d *DocumentationAgent) runNonInteractiveMode(ctx context.Context, prompt string) error {
	fmt.Println("Starting non-interactive documentation update process...")
	fmt.Println("The LLM agent will analyze your package and generate documentation automatically.")
	fmt.Println()

	// First attempt
	result, err := d.executeTaskWithLogging(ctx, prompt)
	if err != nil {
		return err
	}

	// Show the result
	fmt.Println("\n📝 Agent Response:")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Println(result.FinalContent)
	fmt.Println(strings.Repeat("-", 50))

	// Check for token limit messages first - these need special handling
	if isTokenLimitMessage(result.FinalContent) {
		fmt.Println("\n⚠️  LLM hit token limits. Switching to section-based generation...")
		newPrompt, err := d.handleTokenLimitResponse(result.FinalContent)
		if err != nil {
			return fmt.Errorf("failed to handle token limit: %w", err)
		}

		// Retry with section-based approach
		if _, err := d.executeTaskWithLogging(ctx, newPrompt); err != nil {
			return fmt.Errorf("section-based retry failed: %w", err)
		}

		// Check if README was successfully updated after retry
		if updated, err := d.handleReadmeUpdate(); updated {
			fmt.Println("\n📄 README.md was updated successfully with section-based approach!")
			return err
		}
	}

	// Check for errors in response using enhanced detection with conversation context
	if isTaskResultError(result.FinalContent, result.Conversation) {
		fmt.Println("\n❌ Error detected in LLM response.")
		fmt.Println("In non-interactive mode, exiting due to error.")
		return fmt.Errorf("LLM agent encountered an error: %s", result.FinalContent)
	}

	// Check if README was successfully updated
	if updated, err := d.handleReadmeUpdate(); updated {
		fmt.Println("\n📄 README.md was updated successfully!")
		return err
	}

	// Second attempt with specific instructions
	fmt.Println("⚠️  No README.md was updated. Trying again with specific instructions...")
	specificPrompt := "You haven't updated a README.md file yet. Please create the README.md file in the _dev/build/docs/ directory based on your analysis. This is required to complete the task."

	if _, err := d.executeTaskWithLogging(ctx, specificPrompt); err != nil {
		return fmt.Errorf("second attempt failed: %w", err)
	}

	// Final check
	if updated, err := d.handleReadmeUpdate(); updated {
		fmt.Println("\n📄 README.md was updated on second attempt!")
		return err
	}

	return fmt.Errorf("failed to create README.md after two attempts")
}

// runInteractiveMode handles the interactive documentation update flow
func (d *DocumentationAgent) runInteractiveMode(ctx context.Context, prompt string) error {
	fmt.Println("Starting documentation update process...")
	fmt.Println("The LLM agent will analyze your package and update the documentation.")
	fmt.Println()

	for {
		// Execute the task
		result, err := d.executeTaskWithLogging(ctx, prompt)
		if err != nil {
			return err
		}

		// Check for token limit messages first - these need special handling
		if isTokenLimitMessage(result.FinalContent) {
			fmt.Println("\n⚠️  LLM hit token limits. Switching to section-based generation...")
			newPrompt, err := d.handleTokenLimitResponse(result.FinalContent)
			if err != nil {
				return err
			}
			prompt = newPrompt
			continue
		}

		// Handle error responses using enhanced detection with conversation context
		if isTaskResultError(result.FinalContent, result.Conversation) {
			newPrompt, shouldContinue, err := d.handleInteractiveError()
			if err != nil {
				return err
			}
			if !shouldContinue {
				d.restoreOriginalReadme()
				return fmt.Errorf("user chose to exit due to LLM error")
			}
			prompt = newPrompt
			continue
		}

		// Display README content if updated
		readmeUpdated := d.displayReadmeIfUpdated()

		// Get user action
		action, err := d.getUserAction()
		if err != nil {
			return err
		}

		// Handle user action
		newPrompt, shouldContinue, shouldExit, err := d.handleUserAction(action, readmeUpdated)
		if err != nil {
			return err
		}
		if shouldExit {
			return nil
		}
		if shouldContinue {
			prompt = newPrompt
			continue
		}
	}
}

// logAgentResponse logs debug information about the agent response
func (d *DocumentationAgent) logAgentResponse(result *TaskResult) {
	logger.Debugf("DEBUG: Full agent task response follows (may contain sensitive content)")
	logger.Debugf("Agent task response - Success: %t", result.Success)
	logger.Debugf("Agent task response - FinalContent: %s", result.FinalContent)
	logger.Debugf("Agent task response - Conversation entries: %d", len(result.Conversation))
	for i, entry := range result.Conversation {
		logger.Debugf("Agent task response - Conversation[%d]: type=%s, content_length=%d",
			i, entry.Type, len(entry.Content))
		logger.Tracef("Agent task response - Conversation[%d]: content=%s", i, entry.Content)
	}
}

// executeTaskWithLogging executes a task and logs the result
func (d *DocumentationAgent) executeTaskWithLogging(ctx context.Context, prompt string) (*TaskResult, error) {
	fmt.Println("🤖 LLM Agent is working...")

	result, err := d.agent.ExecuteTask(ctx, prompt)
	if err != nil {
		fmt.Println("❌ Agent task failed")
		return nil, fmt.Errorf("agent task failed: %w", err)
	}

	fmt.Println("✅ Task completed")
	d.logAgentResponse(result)
	return result, nil
}

// handleReadmeUpdate checks if README was updated and reports the result
func (d *DocumentationAgent) handleReadmeUpdate() (bool, error) {
	readmeUpdated := d.checkReadmeUpdated()
	if !readmeUpdated {
		return false, nil
	}

	content, err := d.readCurrentReadme()
	if err != nil || content == "" {
		return false, err
	}

	fmt.Printf("✅ Documentation update completed! (%d characters written)\n", len(content))
	return true, nil
}

// handleInteractiveError handles error responses in interactive mode
func (d *DocumentationAgent) handleInteractiveError() (string, bool, error) {
	fmt.Println("\n❌ Error detected in LLM response.")

	errorPrompt := tui.NewSelect("What would you like to do?", []string{
		"Try again",
		"Exit",
	}, "Try again")

	var errorAction string
	err := tui.AskOne(errorPrompt, &errorAction)
	if err != nil {
		return "", false, fmt.Errorf("prompt failed: %w", err)
	}

	if errorAction == "Exit" {
		fmt.Println("⚠️  Exiting due to LLM error.")
		return "", false, nil
	}

	// Continue with retry prompt
	newPrompt := d.buildRevisionPrompt("The previous attempt encountered an error. Please try a different approach to analyze the package and create/update the documentation.")
	return newPrompt, true, nil
}

// handleUserAction processes the user's chosen action
func (d *DocumentationAgent) handleUserAction(action string, readmeUpdated bool) (string, bool, bool, error) {
	switch action {
	case "Accept and finalize":
		return d.handleAcceptAction(readmeUpdated)
	case "Request changes":
		return d.handleRequestChanges()
	case "Cancel":
		fmt.Println("❌ Documentation update cancelled.")
		d.restoreOriginalReadme()
		return "", false, true, nil
	default:
		return "", false, false, fmt.Errorf("unknown action: %s", action)
	}
}

// handleAcceptAction handles the "Accept and finalize" action
func (d *DocumentationAgent) handleAcceptAction(readmeUpdated bool) (string, bool, bool, error) {
	if readmeUpdated {
		// Validate preserved sections if we had original content
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
		return "", false, true, nil
	}

	// README wasn't updated - ask user what to do
	continuePrompt := tui.NewSelect("README.md file wasn't updated. What would you like to do?", []string{
		"Try again",
		"Exit anyway",
	}, "Try again")

	var continueChoice string
	err := tui.AskOne(continuePrompt, &continueChoice)
	if err != nil {
		return "", false, false, fmt.Errorf("prompt failed: %w", err)
	}

	if continueChoice == "Exit anyway" {
		fmt.Println("⚠️  Exiting without creating README.md file.")
		d.restoreOriginalReadme()
		return "", false, true, nil
	}

	fmt.Println("🔄 Trying again to create README.md...")
	newPrompt := d.buildRevisionPrompt("You haven't written a README.md file yet. Please write the README.md file in the _dev/build/docs/ directory based on your analysis.")
	return newPrompt, true, false, nil
}

// handleRequestChanges handles the "Request changes" action
func (d *DocumentationAgent) handleRequestChanges() (string, bool, bool, error) {
	changes, err := tui.AskTextArea("What changes would you like to make to the documentation?")
	if err != nil {
		// Check if user cancelled
		if errors.Is(err, tui.ErrCancelled) {
			fmt.Println("⚠️  Changes request cancelled.")
			return "", true, false, nil // Continue the loop
		}
		return "", false, false, fmt.Errorf("prompt failed: %w", err)
	}

	// Check if no changes were provided
	if strings.TrimSpace(changes) == "" {
		fmt.Println("⚠️  No changes specified. Please try again.")
		return "", true, false, nil // Continue the loop
	}

	newPrompt := d.buildRevisionPrompt(changes)
	return newPrompt, true, false, nil
}

// buildInitialPrompt creates the initial prompt for the LLM
func (d *DocumentationAgent) buildInitialPrompt(manifest *packages.PackageManifest) string {
	return fmt.Sprintf(initialPrompt,
		manifest.Name,
		manifest.Title,
		manifest.Type,
		manifest.Version,
		manifest.Description)
}

// buildRevisionPrompt creates a comprehensive prompt for document revisions that includes all necessary context
func (d *DocumentationAgent) buildRevisionPrompt(changes string) string {
	// Read package manifest for context
	manifest, err := packages.ReadPackageManifestFromPackageRoot(d.packageRoot)
	if err != nil {
		// Fallback to a simpler prompt if we can't read the manifest
		return fmt.Sprintf("Please make the following changes to the documentation:\n\n%s", changes)
	}

	return fmt.Sprintf(revisionPrompt,
		manifest.Name,
		manifest.Title,
		manifest.Type,
		manifest.Version,
		manifest.Description,
		changes)
}

// handleTokenLimitResponse creates a section-based prompt when LLM hits token limits
func (d *DocumentationAgent) handleTokenLimitResponse(originalResponse string) (string, error) {
	// Read package manifest for context
	manifest, err := packages.ReadPackageManifestFromPackageRoot(d.packageRoot)
	if err != nil {
		return "", fmt.Errorf("failed to read package manifest: %w", err)
	}

	// Create a section-based generation prompt
	sectionBasedPrompt := d.buildSectionBasedPrompt(manifest)
	return sectionBasedPrompt, nil
}

// buildSectionBasedPrompt creates a prompt for generating README in sections
func (d *DocumentationAgent) buildSectionBasedPrompt(manifest *packages.PackageManifest) string {
	return fmt.Sprintf(limitHitPrompt,
		manifest.Name,
		manifest.Title,
		manifest.Type,
		manifest.Version,
		manifest.Description)
}

// displayReadmeIfUpdated shows README content if it was updated
func (d *DocumentationAgent) displayReadmeIfUpdated() bool {
	readmeUpdated := d.checkReadmeUpdated()
	if !readmeUpdated {
		fmt.Println("\n⚠️  README.md file not updated")
		return false
	}

	sourceContent, err := d.readCurrentReadme()
	if err != nil || sourceContent == "" {
		fmt.Println("\n⚠️  README.md file exists but could not be read or is empty")
		return false
	}

	// Try to render the content
	renderedContent, shouldBeRendered, err := docs.GenerateReadme("README.md", d.packageRoot)
	if err != nil || !shouldBeRendered {
		fmt.Println("\n⚠️  The generated README.md could not be rendered.")
		fmt.Println("It's recommended that you do not accept this version (ask for revisions or cancel).")
		return true
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

	return true
}

// getUserAction prompts the user for their next action
func (d *DocumentationAgent) getUserAction() (string, error) {
	selectPrompt := tui.NewSelect("What would you like to do?", []string{
		"Accept and finalize",
		"Request changes",
		"Cancel",
	}, "Accept and finalize")

	var action string
	err := tui.AskOne(selectPrompt, &action)
	if err != nil {
		return "", fmt.Errorf("prompt failed: %w", err)
	}

	return action, nil
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
// This is now a wrapper that calls the more sophisticated analysis function
func isErrorResponse(content string) bool {
	// Use the enhanced error detection that considers conversation context
	return isTaskResultError(content, nil)
}

// isTaskResultError provides sophisticated error detection considering conversation context
func isTaskResultError(content string, conversation []ConversationEntry) bool {
	// Empty content is not necessarily an error - it might be after successful tool execution
	if strings.TrimSpace(content) == "" {
		// If we have conversation context, check if recent tools succeeded
		if conversation != nil && hasRecentSuccessfulTools(conversation) {
			return false
		}
		// Empty content without context might indicate a problem, but let's be lenient
		return false
	}

	// Check for token limit messages - these are NOT errors, they're recoverable conditions
	if isTokenLimitMessage(content) {
		return false
	}

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

	// Check for explicit error indicators
	hasErrorIndicator := false
	for _, indicator := range errorIndicators {
		if strings.Contains(contentLower, strings.ToLower(indicator)) {
			hasErrorIndicator = true
			break
		}
	}

	if !hasErrorIndicator {
		return false
	}

	// If we have conversation context and recent tools succeeded, this might be a false error
	if conversation != nil && hasRecentSuccessfulTools(conversation) {
		return false
	}

	return true
}

// isTokenLimitMessage detects if the LLM response indicates it hit token limits
func isTokenLimitMessage(content string) bool {
	tokenLimitIndicators := []string{
		"I reached the maximum response length",
		"maximum response length",
		"reached the token limit",
		"response is too long",
		"breaking this into smaller tasks",
		"due to length constraints",
		"response length limit",
		"token limit reached",
		"output limit exceeded",
		"maximum length exceeded",
	}

	contentLower := strings.ToLower(content)
	for _, indicator := range tokenLimitIndicators {
		if strings.Contains(contentLower, strings.ToLower(indicator)) {
			return true
		}
	}
	return false
}

// hasRecentSuccessfulTools checks if recent tool executions in the conversation were successful
func hasRecentSuccessfulTools(conversation []ConversationEntry) bool {
	// Look at the last few conversation entries for successful tool results
	for i := len(conversation) - 1; i >= 0 && i >= len(conversation)-5; i-- {
		entry := conversation[i]
		if entry.Type == "tool_result" {
			content := strings.ToLower(entry.Content)
			// Check for success indicators
			if strings.Contains(content, "✅ success") ||
				strings.Contains(content, "successfully wrote") ||
				strings.Contains(content, "completed successfully") {
				return true
			}
			// If we hit an actual error, stop looking
			if strings.Contains(content, "❌ error") ||
				strings.Contains(content, "failed:") ||
				strings.Contains(content, "access denied") {
				return false
			}
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

// backupOriginalReadme stores the current README content for potential restoration and comparison to the generated version
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

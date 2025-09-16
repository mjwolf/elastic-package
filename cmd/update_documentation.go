// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/elastic/elastic-package/internal/llmagent"
	"github.com/elastic/elastic-package/internal/packages"
)

const updateDocumentationLongDescription = `Use this command to update package documentation using an AI agent or get manual instructions.

The command supports multiple LLM providers and will automatically use the first available provider based on 
environment variables. It analyzes your package and updates the /_dev/build/docs/README.md file with comprehensive 
documentation based on the package contents and structure.

Environment variables for LLM providers (pick one):
- BEDROCK_API_KEY: API key for Amazon Bedrock
- BEDROCK_REGION: AWS region (defaults to us-east-1)
- GEMINI_API_KEY: API key for Google AI Studio
- GEMINI_MODEL: Model ID (defaults to gemini-2.5-pro)

The AI agent will:
1. Analyze your package structure, data streams, and configuration
2. Generate comprehensive documentation following Elastic's templates
3. Allow you to review and request changes interactively (or automatically accept in non-interactive mode)
4. Create or update the README.md file in /_dev/build/docs/

Use --non-interactive to skip all prompts and automatically accept the first result from the LLM.`

type updateDocumentationAnswers struct {
	Confirm bool
}

func updateDocumentationCommandAction(cmd *cobra.Command, args []string) error {
	cmd.Println("Update package documentation with AI agent")

	packageRoot, found, err := packages.FindPackageRoot()
	if err != nil {
		return fmt.Errorf("locating package root failed: %w", err)
	}
	if !found {
		return errors.New("package root not found, you can only update documentation in the package context")
	}

	cmd.Printf("Package root found: %s\n", packageRoot)

	// Check for non-interactive flag
	nonInteractive, err := cmd.Flags().GetBool("non-interactive")
	if err != nil {
		return fmt.Errorf("failed to get non-interactive flag: %w", err)
	}

	// Check for API key availability for different providers
	bedrockAPIKey := os.Getenv("BEDROCK_API_KEY")
	googleAPIKey := os.Getenv("GEMINI_API_KEY")

	if bedrockAPIKey == "" && googleAPIKey == "" {
		// Use colors to highlight the manual instructions
		yellow := color.New(color.FgYellow)
		cyan := color.New(color.FgCyan)
		green := color.New(color.FgGreen, color.Bold)

		yellow.Println("AI agent is not available (no LLM provider API key set).")
		cmd.Println()
		cyan.Println("To update the documentation manually:")
		green.Println("  1. Edit `_dev/build/docs/README.md`")
		green.Println("  2. Run `elastic-package build`")
		cmd.Println()
		cyan.Println("For AI-powered documentation updates, set one of these environment variables:")
		green.Println("  - BEDROCK_API_KEY (for Amazon Bedrock)")
		green.Println("  - GEMINI_API_KEY (for Google AI Studio)")
		return nil
	}

	// Skip confirmation prompt in non-interactive mode
	if !nonInteractive {
		// Prompt user for confirmation
		qs := []*survey.Question{
			{
				Name: "confirm",
				Prompt: &survey.Confirm{
					Message: "Do you want to update the documentation using the AI agent",
					Default: false,
				},
				Validate: survey.Required,
			},
		}

		var answers updateDocumentationAnswers
		err = survey.Ask(qs, &answers)
		if err != nil {
			return fmt.Errorf("prompt failed: %w", err)
		}

		if !answers.Confirm {
			cmd.Println("Documentation update cancelled.")
			return nil
		}
	} else {
		cmd.Println("Running in non-interactive mode - proceeding automatically.")
	}

	// Create the LLM provider based on available API keys
	var provider llmagent.LLMProvider
	if bedrockAPIKey != "" {
		region := os.Getenv("BEDROCK_REGION")
		if region == "" {
			region = "us-east-1" // Default region
		}
		provider = llmagent.NewBedrockProvider(llmagent.BedrockConfig{
			APIKey: bedrockAPIKey,
			Region: region,
		})
		cmd.Printf("Using Amazon Bedrock provider with region: %s\n", region)
	} else if googleAPIKey != "" {
		modelID := os.Getenv("GEMINI_MODEL")
		if modelID == "" {
			modelID = "gemini-2.5-pro" // Default model
		}
		provider = llmagent.NewGoogleAIStudioProvider(llmagent.GoogleAIStudioConfig{
			APIKey:  googleAPIKey,
			ModelID: modelID,
		})
		cmd.Printf("Using Google AI Studio provider with model: %s\n", modelID)
	}

	// Create the documentation agent
	docAgent, err := llmagent.NewDocumentationAgent(provider, packageRoot)
	if err != nil {
		return fmt.Errorf("failed to create documentation agent: %w", err)
	}

	// Run the documentation update process
	err = docAgent.UpdateDocumentation(cmd.Context(), nonInteractive)
	if err != nil {
		return fmt.Errorf("documentation update failed: %w", err)
	}

	cmd.Println("Done")
	return nil
}

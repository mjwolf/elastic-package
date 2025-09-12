// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"

	"github.com/elastic/elastic-package/internal/llmagent"
	"github.com/elastic/elastic-package/internal/packages"
)

const updateDocumentationLongDescription = `Use this command to update package documentation using an AI agent.

The command uses an agentic LLM to analyze your package and update the /_dev_/docs/README.md file 
with comprehensive documentation based on the package contents and structure.

Requirements:
- BEDROCK_API_KEY environment variable must be set
- BEDROCK_REGION environment variable (optional, defaults to us-east-1)

The AI agent will:
1. Analyze your package structure, data streams, and configuration
2. Generate comprehensive documentation following Elastic's templates
3. Allow you to review and request changes interactively
4. Create or update the README.md file in /_dev_/docs/`

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

	// Check for required environment variables
	apiKey := os.Getenv("BEDROCK_API_KEY")
	if apiKey == "" {
		return errors.New("BEDROCK_API_KEY environment variable is required")
	}

	region := os.Getenv("BEDROCK_REGION")
	if region == "" {
		region = "us-east-1" // Default region
	}

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

	// Create the LLM provider
	provider := llmagent.NewBedrockProvider(llmagent.BedrockConfig{
		APIKey: apiKey,
		Region: region,
	})

	// Create the documentation agent
	docAgent, err := llmagent.NewDocumentationAgent(provider, packageRoot)
	if err != nil {
		return fmt.Errorf("failed to create documentation agent: %w", err)
	}

	// Run the documentation update process
	err = docAgent.UpdateDocumentation(cmd.Context())
	if err != nil {
		return fmt.Errorf("documentation update failed: %w", err)
	}

	cmd.Println("Done")
	return nil
}

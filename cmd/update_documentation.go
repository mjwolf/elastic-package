// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/elastic/elastic-package/internal/cobraext"
	"github.com/elastic/elastic-package/internal/llmagent"
	"github.com/elastic/elastic-package/internal/packages"
	"github.com/elastic/elastic-package/internal/profile"
	"github.com/elastic/elastic-package/internal/tui"
)

const updateDocumentationLongDescription = `Use this command to update package documentation using an AI agent or to get manual instructions for update.

The AI agent will:
1. Analyze your package structure, data streams, and configuration
2. Generate comprehensive documentation following Elastic's templates
3. Allow you to review and request changes interactively (or automatically accept in non-interactive mode)
4. Create or update the README.md file in /_dev/build/docs/

After the AI agent has generated updated documentation, you will be able to review it, and optionally, provide additional prompts that will be given
to the AI agent to request changes to the generated documentation.

Use --non-interactive to skip all prompts and automatically accept the first result from the LLM.

If no LLM provider is configured, this command will print instructions for updating the documentation manually.

The command supports multiple LLM providers and will automatically use the first available provider based on 
environment variables or profile configuration. It analyzes your package and updates the /_dev/build/docs/README.md file with comprehensive 
documentation based on the package contents and structure.

Configuration options for LLM providers (environment variables or profile config):
- BEDROCK_API_KEY / llm.bedrock.api_key: API key for Amazon Bedrock
- BEDROCK_REGION / llm.bedrock.region: AWS region (defaults to us-east-1)
- BEDROCK_MODEL / llm.bedrock.model: Model ID (defaults to anthropic.claude-3-5-sonnet-20241022-v2:0)
- GEMINI_API_KEY / llm.gemini.api_key: API key for Gemini
- GEMINI_MODEL / llm.gemini.model: Model ID (defaults to gemini-2.5-pro)
- LOCAL_LLM_ENDPOINT / llm.local.endpoint: Endpoint for local LLM server
- LOCAL_LLM_MODEL / llm.local.model: Model name for local LLM (defaults to llama2)
- LOCAL_LLM_API_KEY / llm.local.api_key: API key for local LLM (optional)`



// getConfigValue retrieves a configuration value with fallback from environment variable to profile config
func getConfigValue(profile *profile.Profile, envVar, configKey, defaultValue string) string {
	// First check environment variable
	if envValue := os.Getenv(envVar); envValue != "" {
		return envValue
	}

	// Then check profile configuration
	if profile != nil {
		return profile.Config(configKey, defaultValue)
	}

	return defaultValue
}

func updateDocumentationCommandAction(cmd *cobra.Command, args []string) error {
	packageRoot, found, err := packages.FindPackageRoot()
	if err != nil {
		return fmt.Errorf("locating package root failed: %w", err)
	}
	if !found {
		return errors.New("package root not found, you can only update documentation in the package context")
	}

	// Check for non-interactive flag
	nonInteractive, err := cmd.Flags().GetBool("non-interactive")
	if err != nil {
		return fmt.Errorf("failed to get non-interactive flag: %w", err)
	}

	// Get profile for configuration access
	profile, err := cobraext.GetProfileFlag(cmd)
	if err != nil {
		return fmt.Errorf("failed to get profile: %w", err)
	}

	// Check for API key availability for different providers (environment variables take precedence over profile config)
	bedrockAPIKey := getConfigValue(profile, "BEDROCK_API_KEY", "llm.bedrock.api_key", "")
	googleAPIKey := getConfigValue(profile, "GEMINI_API_KEY", "llm.gemini.api_key", "")
	localEndpoint := getConfigValue(profile, "LOCAL_LLM_ENDPOINT", "llm.local.endpoint", "")

	if bedrockAPIKey == "" && googleAPIKey == "" && localEndpoint == "" {
		// Use standardized TUI colors for consistent output
		cmd.Println(tui.Warning("AI agent is not available (no LLM provider API key set)."))
		cmd.Println()
		cmd.Println(tui.Info("To update the documentation manually:"))
		cmd.Println(tui.Info("  1. Edit `_dev/build/docs/README.md`. Please follow the documentation guidelines from https://www.elastic.co/docs/extend/integrations/documentation-guidelines."))
		cmd.Println(tui.Info("  2. Run `elastic-package build`"))
		cmd.Println()
		cmd.Println(tui.Info("For AI-powered documentation updates, configure one of these LLM providers:"))
		cmd.Println(tui.Info("  - Amazon Bedrock: Set BEDROCK_API_KEY or add llm.bedrock.api_key to profile config"))
		cmd.Println(tui.Info("  - Gemini: Set GEMINI_API_KEY or add llm.gemini.api_key to profile config"))
		cmd.Println(tui.Info("  - Local LLM: Set LOCAL_LLM_ENDPOINT or add llm.local.endpoint to profile config"))
		cmd.Println()
		cmd.Println(tui.Info("Profile configuration: ~/.elastic-package/profiles/<profile>/config.yml"))
		return nil
	}

	// Skip confirmation prompt in non-interactive mode
	if !nonInteractive {
		// Prompt user for confirmation
		confirmPrompt := tui.NewConfirm("Do you want to update the documentation using the AI agent?", true)

		var confirm bool
		err = tui.AskOne(confirmPrompt, &confirm, tui.Required)
		if err != nil {
			return fmt.Errorf("prompt failed: %w", err)
		}

		if !confirm {
			cmd.Println("Documentation update cancelled.")
			return nil
		}
	} else {
		cmd.Println("Running in non-interactive mode - proceeding automatically.")
	}

	// Create the LLM provider based on available API keys/endpoints
	var provider llmagent.LLMProvider
	if bedrockAPIKey != "" {
		region := getConfigValue(profile, "BEDROCK_REGION", "llm.bedrock.region", "us-east-1")
		modelID := getConfigValue(profile, "BEDROCK_MODEL", "llm.bedrock.model", "anthropic.claude-3-5-sonnet-20241022-v2:0")
		provider = llmagent.NewBedrockProvider(llmagent.BedrockConfig{
			APIKey:  bedrockAPIKey,
			Region:  region,
			ModelID: modelID,
			MaxTokens: 4096,
		})
		cmd.Printf("Using Amazon Bedrock provider with region: %s, model: %s\n", region, modelID)
	} else if googleAPIKey != "" {
		modelID := getConfigValue(profile, "GEMINI_MODEL", "llm.gemini.model", "gemini-2.5-pro")
		provider = llmagent.NewGeminiProvider(llmagent.GeminiConfig{
			APIKey:  googleAPIKey,
			ModelID: modelID,
		})
		cmd.Printf("Using Gemini provider with model: %s\n", modelID)
	} else if localEndpoint != "" {
		modelID := getConfigValue(profile, "LOCAL_LLM_MODEL", "llm.local.model", "llama2")
		localAPIKey := getConfigValue(profile, "LOCAL_LLM_API_KEY", "llm.local.api_key", "")
		provider = llmagent.NewLocalProvider(llmagent.LocalConfig{
			Endpoint: localEndpoint,
			ModelID:  modelID,
			APIKey:   localAPIKey,
		})
		cmd.Printf("Using Local LLM provider with endpoint: %s, model: %s\n", localEndpoint, modelID)
	} else {
		return fmt.Errorf("Unknown LLM provider selected")
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

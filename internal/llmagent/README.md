# LLM Agent for Documentation Updates

This package provides an agentic LLM system for automatically updating Elastic package documentation.

## Architecture

### Provider Interface (`provider.go`)
- **LLMProvider**: Extensible interface for different LLM providers
- **LLMResponse**: Standardized response format
- **Tool**: Interface for tools that the LLM can use
- **ToolCall**: Represents tool invocations from the LLM

### Tools (`tools.go`)
The LLM agent has access to these tools within the package directory:
- **list_directory**: Lists files and directories
- **read_file**: Reads file contents  
- **write_file**: Writes content to files

All tools are scoped to the package directory for security.

### Bedrock Provider (`bedrock.go`)
Implementation for Amazon Bedrock:
- Supports Claude models (default: `anthropic.claude-3-5-sonnet-20240620-v1:0`)
- Configurable region (default: `us-east-1`)
- Requires API key authentication

### Gemini Provider (`gemini.go`)
Implementation for Gemini:
- Supports Gemini models (default: `gemini-2.5-pro`)
- Uses Google's Generative Language API
- Requires API key authentication

### Local LLM Provider (`local.go`)
Implementation for local LLM servers:
- Supports OpenAI-compatible local servers (Ollama, LocalAI, etc.)
- Configurable endpoint (default: `http://localhost:11434`)
- Configurable model name (default: `llama2`)
- Optional API key support for servers that require authentication

### Agent (`agent.go`)
Core agent that:
- Manages conversation with the LLM
- Executes tool calls
- Handles iterative task completion
- Prevents infinite loops

### Documentation Agent (`docagent.go`)
Specialized agent for documentation tasks:
- Analyzes package structure and contents
- Uses the package template (`package-docs-readme.md.tmpl`)
- Interactive loop for user feedback
- Creates/updates `/_dev/build/docs/README.md`

## Usage

### Environment Variables

#### For Amazon Bedrock:
```bash
export BEDROCK_API_KEY="your-bedrock-api-key"
export BEDROCK_REGION="us-east-1"  # optional
```

#### For Gemini:
```bash
export GEMINI_API_KEY="your-google-api-key"
export GEMINI_MODEL="gemini-2.5-pro"  # optional
```

#### For Local LLM (Ollama, LocalAI, etc.):
```bash
export LOCAL_LLM_ENDPOINT="http://localhost:11434"  # required
export LOCAL_LLM_MODEL="llama2"  # optional, defaults to llama2
export LOCAL_LLM_API_KEY="your-api-key"  # optional, for servers requiring auth
```

### Profile Configuration

As an alternative to environment variables, you can configure LLM providers in your elastic-package profile configuration:

```yaml
# ~/.elastic-package/profiles/<profile>/config.yml

## Amazon Bedrock
llm.bedrock.api_key: "your-bedrock-api-key"
llm.bedrock.region: "us-east-1"  # optional, defaults to us-east-1
llm.bedrock.model: "anthropic.claude-3-5-sonnet-20241022-v2:0"  # optional

## Gemini
llm.gemini.api_key: "your-google-api-key"
llm.gemini.model: "gemini-2.5-pro"  # optional, defaults to gemini-2.5-pro

## Local LLM Provider
llm.local.endpoint: "http://localhost:11434"  # required for local LLM
llm.local.model: "llama2"  # optional, defaults to llama2
llm.local.api_key: "your-api-key"  # optional, for servers requiring auth
```

**Configuration Priority:**
1. Environment variables (highest priority)
2. Profile configuration
3. Default values (lowest priority)

### Command
```bash
elastic-package update documentation
```

### Interactive Process
1. Agent analyzes the package structure
2. Generates documentation based on findings
3. Shows preview to user
4. User can:
   - Accept and finalize
   - Request specific changes
   - Cancel the process

## Extensibility

### Using Existing Providers

#### Bedrock Provider:
```go
provider := llmagent.NewBedrockProvider(llmagent.BedrockConfig{
    APIKey: "your-api-key",
    Region: "us-east-1",
    ModelID: "anthropic.claude-3-5-sonnet-20240620-v1:0",
})
```

#### Gemini Provider:
```go
provider := llmagent.NewGeminiProvider(llmagent.GeminiConfig{
    APIKey: "your-api-key", 
    ModelID: "gemini-2.5-pro",
})
```

#### Local LLM Provider:
```go
provider := llmagent.NewLocalProvider(llmagent.LocalConfig{
    Endpoint: "http://localhost:11434",
    ModelID: "llama2", 
    APIKey: "", // Optional
})
```

### Adding New LLM Providers
1. Implement the `LLMProvider` interface
2. Handle the provider-specific API communication
3. Convert responses to the standard `LLMResponse` format

Example:
```go
type OpenAIProvider struct {
    // provider-specific fields
}

func (o *OpenAIProvider) GenerateResponse(ctx context.Context, prompt string, tools []Tool) (*LLMResponse, error) {
    // implementation
}
```

### Adding New Tools
1. Define the tool in the tools slice
2. Implement the `ToolHandler` function
3. Ensure proper security constraints

Example:
```go
Tool{
    Name: "new_tool",
    Description: "Description of what this tool does",
    Parameters: map[string]interface{}{
        // JSON schema for parameters
    },
    Handler: func(ctx context.Context, arguments string) (*ToolResult, error) {
        // implementation
    },
}
```

## Security

- All file operations are restricted to the package directory
- Path traversal attacks are prevented
- Tools validate input parameters
- API keys are handled securely via environment variables

## Future Enhancements

- Support for additional LLM providers (OpenAI, Anthropic, local models)
- Additional tools for package analysis
- Templates for different documentation types
- Batch processing for multiple packages
- Integration with CI/CD pipelines

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
- Creates/updates `/_dev_/docs/README.md`

## Usage

### Environment Variables
```bash
export BEDROCK_API_KEY="your-api-key"
export BEDROCK_REGION="us-east-1"  # optional
```

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

- Support for other LLM providers (OpenAI, Anthropic, local models)
- Additional tools for package analysis
- Templates for different documentation types
- Batch processing for multiple packages
- Integration with CI/CD pipelines

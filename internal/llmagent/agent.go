// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package llmagent

import (
	"context"
	"fmt"
	"strings"
)

// Agent represents an LLM agent that can use tools
type Agent struct {
	provider LLMProvider
	tools    []Tool
}

// NewAgent creates a new LLM agent
func NewAgent(provider LLMProvider, tools []Tool) *Agent {
	return &Agent{
		provider: provider,
		tools:    tools,
	}
}

// ExecuteTask runs the agent to complete a task
func (a *Agent) ExecuteTask(ctx context.Context, prompt string) (*TaskResult, error) {
	var conversation []ConversationEntry

	// Add initial prompt
	conversation = append(conversation, ConversationEntry{
		Type:    "user",
		Content: prompt,
	})

	// Adjust max iterations based on provider stability
	maxIterations := 15 // Increased from 10 to handle unstable models like gemini-2.5-flash
	if strings.Contains(strings.ToLower(a.provider.Name()), "gemini") {
		maxIterations = 20 // Additional iterations for Gemini models due to known instability
	}
	for i := 0; i < maxIterations; i++ {
		// Build the full prompt with conversation history
		fullPrompt := a.buildPrompt(conversation)

		// Get response from LLM
		response, err := a.provider.GenerateResponse(ctx, fullPrompt, a.tools)
		if err != nil {
			return nil, fmt.Errorf("failed to get LLM response: %w", err)
		}

		// Add LLM response to conversation
		conversation = append(conversation, ConversationEntry{
			Type:    "assistant",
			Content: response.Content,
		})

		// If there are tool calls, execute them
		if len(response.ToolCalls) > 0 {
			for _, toolCall := range response.ToolCalls {
				result, err := a.executeTool(ctx, toolCall)
				if err != nil {
					conversation = append(conversation, ConversationEntry{
						Type:    "tool_result",
						Content: fmt.Sprintf("Tool %s failed: %v", toolCall.Name, err),
					})
				} else {
					if result.Error != "" {
						conversation = append(conversation, ConversationEntry{
							Type:    "tool_result",
							Content: fmt.Sprintf("Tool %s error: %s", toolCall.Name, result.Error),
						})
					} else {
						conversation = append(conversation, ConversationEntry{
							Type:    "tool_result",
							Content: fmt.Sprintf("Tool %s result: %s", toolCall.Name, result.Content),
						})
					}
				}
			}
		} else if response.Finished {
			// No tool calls and LLM indicated it's finished
			return &TaskResult{
				Success:      true,
				FinalContent: response.Content,
				Conversation: conversation,
			}, nil
		} else {
			// No tool calls and not finished - this can happen with unstable models
			// Add a prompt to encourage the LLM to complete the task or use tools
			conversation = append(conversation, ConversationEntry{
				Type:    "user",
				Content: "Please complete the task or use the available tools to gather the information you need. If the task is complete, please indicate that you are finished.",
			})
		}
	}

	return &TaskResult{
		Success:      false,
		FinalContent: "Task did not complete within maximum iterations",
		Conversation: conversation,
	}, nil
}

// executeTool executes a specific tool call
func (a *Agent) executeTool(ctx context.Context, toolCall ToolCall) (*ToolResult, error) {
	// Find the tool
	for _, tool := range a.tools {
		if tool.Name == toolCall.Name {
			return tool.Handler(ctx, toolCall.Arguments)
		}
	}

	return nil, fmt.Errorf("tool not found: %s", toolCall.Name)
}

// buildPrompt creates the full prompt with conversation history
func (a *Agent) buildPrompt(conversation []ConversationEntry) string {
	var builder strings.Builder

	for _, entry := range conversation {
		switch entry.Type {
		case "user":
			builder.WriteString("Human: ")
			builder.WriteString(entry.Content)
			builder.WriteString("\n\n")
		case "assistant":
			builder.WriteString("Assistant: ")
			builder.WriteString(entry.Content)
			builder.WriteString("\n\n")
		case "tool_result":
			builder.WriteString("Tool Result: ")
			builder.WriteString(entry.Content)
			builder.WriteString("\n\n")
		}
	}

	return builder.String()
}

// TaskResult represents the result of a task execution
type TaskResult struct {
	Success      bool
	FinalContent string
	Conversation []ConversationEntry
}

// ConversationEntry represents an entry in the conversation
type ConversationEntry struct {
	Type    string // "user", "assistant", "tool_result"
	Content string
}

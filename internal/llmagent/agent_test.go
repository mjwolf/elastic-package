// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package llmagent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestNewAgent(t *testing.T) {
	provider := NewMockLLMProvider(SimpleConversation)
	tools := []Tool{
		{
			Name:        "test_tool",
			Description: "A test tool",
			Parameters:  map[string]interface{}{},
			Handler: func(ctx context.Context, arguments string) (*ToolResult, error) {
				return &ToolResult{Content: "test"}, nil
			},
		},
	}

	agent := NewAgent(provider, tools)

	if agent == nil {
		t.Fatal("NewAgent should return a non-nil agent")
	}
	if agent.provider != provider {
		t.Error("Agent should use the provided provider")
	}
	if len(agent.tools) != len(tools) {
		t.Errorf("Expected %d tools, got %d", len(tools), len(agent.tools))
	}
	if agent.tools[0].Name != tools[0].Name {
		t.Error("Agent should use the provided tools")
	}
}

func TestAgent_ExecuteTask_SimpleConversation(t *testing.T) {
	provider := NewMockLLMProvider(SimpleConversation)
	agent := NewAgent(provider, []Tool{})

	ctx := context.Background()
	result, err := agent.ExecuteTask(ctx, "Hello, how are you?")

	if err != nil {
		t.Fatalf("ExecuteTask failed: %v", err)
	}

	if !result.Success {
		t.Error("Task should succeed")
	}

	if result.FinalContent == "" {
		t.Error("Result should have content")
	}

	if len(result.Conversation) == 0 {
		t.Error("Conversation should have entries")
	}

	// Check conversation structure
	if result.Conversation[0].Type != "user" {
		t.Error("First conversation entry should be user")
	}
	if result.Conversation[0].Content != "Hello, how are you?" {
		t.Error("First conversation entry should contain original prompt")
	}

	// Verify provider was called
	if err := provider.AssertCallCount(1); err != nil {
		t.Error(err)
	}
}

func TestAgent_ExecuteTask_MultiTurnWithTools(t *testing.T) {
	provider := NewMockLLMProvider(MultiTurnWithTools)

	// Create a test tool
	testTool := Tool{
		Name:        "read_file",
		Description: "Read a file",
		Parameters:  map[string]interface{}{},
		Handler: func(ctx context.Context, arguments string) (*ToolResult, error) {
			return &ToolResult{Content: "File content here"}, nil
		},
	}

	agent := NewAgent(provider, []Tool{testTool})

	ctx := context.Background()
	result, err := agent.ExecuteTask(ctx, "Read the test file for me")

	if err != nil {
		t.Fatalf("ExecuteTask failed: %v", err)
	}

	if !result.Success {
		t.Error("Task should succeed")
	}

	// Should have multiple conversation turns
	if len(result.Conversation) < 3 {
		t.Errorf("Expected at least 3 conversation entries (user, assistant, tool_result), got %d", len(result.Conversation))
	}

	// Check for tool_result entry
	foundToolResult := false
	for _, entry := range result.Conversation {
		if entry.Type == "tool_result" {
			foundToolResult = true
			if !strings.Contains(entry.Content, "read_file") {
				t.Error("Tool result should mention the tool name")
			}
			break
		}
	}
	if !foundToolResult {
		t.Error("Should have tool_result conversation entry")
	}

	// Verify provider was called multiple times
	if provider.GetCallCount() < 2 {
		t.Errorf("Expected multiple provider calls, got %d", provider.GetCallCount())
	}
}

func TestAgent_ExecuteTask_ToolNotFound(t *testing.T) {
	// Create a provider that tries to call a non-existent tool
	provider := NewMockLLMProvider(SimpleConversation)
	// Override the response to call a non-existent tool
	provider.scenario = MultiTurnWithTools

	agent := NewAgent(provider, []Tool{}) // No tools provided

	ctx := context.Background()
	result, err := agent.ExecuteTask(ctx, "Use a non-existent tool")

	if err != nil {
		t.Fatalf("ExecuteTask should not return error: %v", err)
	}

	// Task should continue even if tool is not found
	if len(result.Conversation) == 0 {
		t.Error("Should have conversation entries")
	}

	// Should have a tool_result entry with error
	foundToolError := false
	for _, entry := range result.Conversation {
		if entry.Type == "tool_result" && strings.Contains(entry.Content, "failed") {
			foundToolError = true
			break
		}
	}
	if !foundToolError {
		t.Error("Should have tool error in conversation")
	}
}

func TestAgent_ExecuteTask_ToolHandlerError(t *testing.T) {
	provider := NewMockLLMProvider(MultiTurnWithTools)

	// Create a tool that returns an error
	errorTool := Tool{
		Name:        "read_file",
		Description: "Read a file",
		Parameters:  map[string]interface{}{},
		Handler: func(ctx context.Context, arguments string) (*ToolResult, error) {
			return nil, fmt.Errorf("file access denied")
		},
	}

	agent := NewAgent(provider, []Tool{errorTool})

	ctx := context.Background()
	result, err := agent.ExecuteTask(ctx, "Read the file")

	if err != nil {
		t.Fatalf("ExecuteTask should handle tool errors gracefully: %v", err)
	}

	// Should have a tool_result entry with error
	foundToolError := false
	for _, entry := range result.Conversation {
		if entry.Type == "tool_result" && strings.Contains(entry.Content, "file access denied") {
			foundToolError = true
			break
		}
	}
	if !foundToolError {
		t.Error("Should have tool error in conversation")
	}
}

func TestAgent_ExecuteTask_ToolResultError(t *testing.T) {
	provider := NewMockLLMProvider(MultiTurnWithTools)

	// Create a tool that returns a ToolResult with error
	errorTool := Tool{
		Name:        "read_file",
		Description: "Read a file",
		Parameters:  map[string]interface{}{},
		Handler: func(ctx context.Context, arguments string) (*ToolResult, error) {
			return &ToolResult{
				Content: "",
				Error:   "Permission denied",
			}, nil
		},
	}

	agent := NewAgent(provider, []Tool{errorTool})

	ctx := context.Background()
	result, err := agent.ExecuteTask(ctx, "Read the file")

	if err != nil {
		t.Fatalf("ExecuteTask should handle tool result errors gracefully: %v", err)
	}

	// Should have a tool_result entry with error
	foundToolError := false
	for _, entry := range result.Conversation {
		if entry.Type == "tool_result" && strings.Contains(entry.Content, "Permission denied") {
			foundToolError = true
			break
		}
	}
	if !foundToolError {
		t.Error("Should have tool result error in conversation")
	}
}

func TestAgent_ExecuteTask_MaxIterations(t *testing.T) {
	// Create a provider that never finishes
	provider := NewMockLLMProvider(SimpleConversation)
	provider.scenario = MultiTurnWithTools // Will keep making tool calls

	// Create a tool that always succeeds
	infiniteTool := Tool{
		Name:        "read_file",
		Description: "Read a file",
		Parameters:  map[string]interface{}{},
		Handler: func(ctx context.Context, arguments string) (*ToolResult, error) {
			return &ToolResult{Content: "File content"}, nil
		},
	}

	agent := NewAgent(provider, []Tool{infiniteTool})

	// Modify the mock to never finish
	originalProvider := provider
	alwaysContinueProvider := &MockLLMProvider{
		scenario:         MultiTurnWithTools,
		packageStructure: originalProvider.packageStructure,
		workflowState:    originalProvider.workflowState,
		calls:            make([]MockCall, 0),
		maxIterations:    15,
	}

	// Override generateSimpleResponse to never finish
	agent.provider = &neverFinishProvider{mock: alwaysContinueProvider}

	ctx := context.Background()
	result, err := agent.ExecuteTask(ctx, "Keep going forever")

	if err != nil {
		t.Fatalf("ExecuteTask should not return error: %v", err)
	}

	if result.Success {
		t.Error("Task should not succeed when hitting max iterations")
	}

	if !strings.Contains(result.FinalContent, "maximum iterations") {
		t.Error("Should indicate max iterations reached")
	}

	if len(result.Conversation) < 15 {
		t.Error("Should have many conversation entries")
	}
}

// neverFinishProvider is a test helper that never marks responses as finished
type neverFinishProvider struct {
	mock      *MockLLMProvider
	callCount int
}

func (n *neverFinishProvider) Name() string {
	return "Never Finish Provider"
}

func (n *neverFinishProvider) GenerateResponse(ctx context.Context, prompt string, tools []Tool) (*LLMResponse, error) {
	n.callCount++

	// Always return a response that's not finished and has no tool calls
	// This will trigger the "encourage completion" logic
	return &LLMResponse{
		Content:   fmt.Sprintf("Response %d - continuing...", n.callCount),
		ToolCalls: []ToolCall{},
		Finished:  false,
	}, nil
}

func TestAgent_ExecuteTask_ContextCancellation(t *testing.T) {
	provider := NewMockLLMProvider(SimpleConversation)
	agent := NewAgent(provider, []Tool{})

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// The provider should be called and may succeed or fail depending on timing
	// The important thing is that the function returns promptly
	start := time.Now()
	result, err := agent.ExecuteTask(ctx, "Hello")
	elapsed := time.Since(start)

	// Should return quickly (within 1 second)
	if elapsed > time.Second {
		t.Errorf("ExecuteTask took too long with cancelled context: %v", elapsed)
	}

	// Either result should be valid or error should be context-related
	if err != nil {
		if !strings.Contains(err.Error(), "context") {
			t.Errorf("Expected context-related error, got: %v", err)
		}
	} else if result == nil {
		t.Error("Should return either result or error")
	}
}

func TestAgent_ExecuteTool(t *testing.T) {
	tests := []struct {
		name        string
		tools       []Tool
		toolCall    ToolCall
		wantContent string
		wantError   bool
	}{
		{
			name: "successful tool execution",
			tools: []Tool{
				{
					Name: "test_tool",
					Handler: func(ctx context.Context, arguments string) (*ToolResult, error) {
						return &ToolResult{Content: "success"}, nil
					},
				},
			},
			toolCall: ToolCall{
				ID:        "call_1",
				Name:      "test_tool",
				Arguments: `{"param": "value"}`,
			},
			wantContent: "success",
			wantError:   false,
		},
		{
			name: "tool not found",
			tools: []Tool{
				{
					Name: "other_tool",
					Handler: func(ctx context.Context, arguments string) (*ToolResult, error) {
						return &ToolResult{Content: "other"}, nil
					},
				},
			},
			toolCall: ToolCall{
				ID:        "call_1",
				Name:      "missing_tool",
				Arguments: `{"param": "value"}`,
			},
			wantError: true,
		},
		{
			name: "tool handler error",
			tools: []Tool{
				{
					Name: "error_tool",
					Handler: func(ctx context.Context, arguments string) (*ToolResult, error) {
						return nil, fmt.Errorf("handler error")
					},
				},
			},
			toolCall: ToolCall{
				ID:        "call_1",
				Name:      "error_tool",
				Arguments: `{"param": "value"}`,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewMockLLMProvider(SimpleConversation)
			agent := NewAgent(provider, tt.tools)

			result, err := agent.executeTool(context.Background(), tt.toolCall)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Error("Expected result but got nil")
				return
			}

			if result.Content != tt.wantContent {
				t.Errorf("Expected content '%s', got '%s'", tt.wantContent, result.Content)
			}
		})
	}
}

func TestAgent_BuildPrompt(t *testing.T) {
	provider := NewMockLLMProvider(SimpleConversation)
	agent := NewAgent(provider, []Tool{})

	tests := []struct {
		name         string
		conversation []ConversationEntry
		wantContains []string
	}{
		{
			name: "single user message",
			conversation: []ConversationEntry{
				{Type: "user", Content: "Hello"},
			},
			wantContains: []string{"Human: Hello"},
		},
		{
			name: "full conversation",
			conversation: []ConversationEntry{
				{Type: "user", Content: "Hello"},
				{Type: "assistant", Content: "Hi there!"},
				{Type: "tool_result", Content: "Tool result: success"},
			},
			wantContains: []string{
				"Human: Hello",
				"Assistant: Hi there!",
				"Tool Result: Tool result: success",
			},
		},
		{
			name:         "empty conversation",
			conversation: []ConversationEntry{},
			wantContains: []string{},
		},
		{
			name: "unknown type",
			conversation: []ConversationEntry{
				{Type: "unknown", Content: "Should be ignored"},
			},
			wantContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := agent.buildPrompt(tt.conversation)

			for _, want := range tt.wantContains {
				if !strings.Contains(prompt, want) {
					t.Errorf("Prompt should contain '%s'\nPrompt: %s", want, prompt)
				}
			}

			// For empty conversation, prompt should be empty
			if len(tt.conversation) == 0 && prompt != "" {
				t.Errorf("Empty conversation should produce empty prompt, got: %s", prompt)
			}
		})
	}
}

func TestAgent_ExecuteTask_ProviderError(t *testing.T) {
	provider := NewMockLLMProvider(SimpleConversation)
	provider.SetError(true, "provider connection failed")

	agent := NewAgent(provider, []Tool{})

	ctx := context.Background()
	result, err := agent.ExecuteTask(ctx, "Hello")

	if err == nil {
		t.Error("Expected error from provider")
		return
	}

	if !strings.Contains(err.Error(), "failed to get LLM response") {
		t.Errorf("Expected LLM response error, got: %v", err)
	}

	if !strings.Contains(err.Error(), "provider connection failed") {
		t.Errorf("Error should contain original provider error: %v", err)
	}

	if result != nil {
		t.Error("Should not return result when provider fails")
	}
}

func TestAgent_ExecuteTask_EmptyPrompt(t *testing.T) {
	provider := NewMockLLMProvider(SimpleConversation)
	agent := NewAgent(provider, []Tool{})

	ctx := context.Background()
	result, err := agent.ExecuteTask(ctx, "")

	if err != nil {
		t.Fatalf("ExecuteTask should handle empty prompt: %v", err)
	}

	if !result.Success {
		t.Error("Should succeed with empty prompt")
	}

	// Should still have conversation entry
	if len(result.Conversation) == 0 {
		t.Error("Should have conversation entries even with empty prompt")
	}

	if result.Conversation[0].Content != "" {
		t.Error("First conversation entry should have empty content")
	}
}

func TestAgent_IsTaskLikelyComplete(t *testing.T) {
	provider := &MockLLMProvider{}
	agent := NewAgent(provider, []Tool{})

	tests := []struct {
		name         string
		conversation []ConversationEntry
		expected     bool
	}{
		{
			name: "successful README write with completion language",
			conversation: []ConversationEntry{
				{Type: "user", Content: "Please update the documentation"},
				{Type: "assistant", Content: "I'll update the README.md file"},
				{Type: "tool_result", Content: "Tool write_file result: Successfully wrote 1234 bytes to _dev/build/docs/README.md"},
				{Type: "assistant", Content: "The documentation has been updated successfully. The README is now complete."},
			},
			expected: true,
		},
		{
			name: "successful write but no completion language",
			conversation: []ConversationEntry{
				{Type: "user", Content: "Please update the documentation"},
				{Type: "tool_result", Content: "Tool write_file result: Successfully wrote 1234 bytes to _dev/build/docs/README.md"},
				{Type: "assistant", Content: "I wrote to the file."},
			},
			expected: false,
		},
		{
			name: "completion language but no successful writes",
			conversation: []ConversationEntry{
				{Type: "user", Content: "Please update the documentation"},
				{Type: "assistant", Content: "The task is completed and finished successfully."},
			},
			expected: false,
		},
		{
			name: "successful write with error",
			conversation: []ConversationEntry{
				{Type: "user", Content: "Please update the documentation"},
				{Type: "tool_result", Content: "Tool write_file result: Successfully wrote 1234 bytes to _dev/build/docs/README.md"},
				{Type: "tool_result", Content: "Tool read_file error: File not found"},
				{Type: "assistant", Content: "The documentation update is complete."},
			},
			expected: false,
		},
		{
			name:         "empty conversation",
			conversation: []ConversationEntry{},
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agent.isTaskLikelyComplete(tt.conversation)
			if result != tt.expected {
				t.Errorf("isTaskLikelyComplete() = %t, want %t", result, tt.expected)
			}
		})
	}
}

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package llmagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewGeminiProvider(t *testing.T) {
	tests := []struct {
		name             string
		config           GeminiConfig
		expectedModel    string
		expectedEndpoint string
	}{
		{
			name: "default configuration",
			config: GeminiConfig{
				APIKey: "test-api-key",
			},
			expectedModel:    "gemini-2.5-pro",
			expectedEndpoint: "https://generativelanguage.googleapis.com/v1beta",
		},
		{
			name: "custom configuration",
			config: GeminiConfig{
				APIKey:   "custom-key",
				ModelID:  "gemini-1.5-pro",
				Endpoint: "https://custom.endpoint.com/v1",
			},
			expectedModel:    "gemini-1.5-pro",
			expectedEndpoint: "https://custom.endpoint.com/v1",
		},
		{
			name: "partial configuration",
			config: GeminiConfig{
				APIKey:  "partial-key",
				ModelID: "custom-model",
			},
			expectedModel:    "custom-model",
			expectedEndpoint: "https://generativelanguage.googleapis.com/v1beta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewGeminiProvider(tt.config)

			if provider == nil {
				t.Fatal("NewGeminiProvider should return a non-nil provider")
			}

			if provider.Name() != "Gemini" {
				t.Errorf("Expected provider name 'Gemini', got '%s'", provider.Name())
			}

			if provider.modelID != tt.expectedModel {
				t.Errorf("Expected model ID '%s', got '%s'", tt.expectedModel, provider.modelID)
			}

			if provider.endpoint != tt.expectedEndpoint {
				t.Errorf("Expected endpoint '%s', got '%s'", tt.expectedEndpoint, provider.endpoint)
			}

			if provider.apiKey != tt.config.APIKey {
				t.Errorf("Expected API key '%s', got '%s'", tt.config.APIKey, provider.apiKey)
			}

			if provider.client == nil {
				t.Error("HTTP client should be initialized")
			}

			if provider.client.Timeout != 60*time.Second {
				t.Errorf("Expected timeout 60s, got %v", provider.client.Timeout)
			}
		})
	}
}

func TestGeminiProvider_GenerateResponse_Success(t *testing.T) {
	// Create a test server that returns a successful Gemini API response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and headers
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		if r.Header.Get("X-goog-api-key") == "" {
			t.Error("Expected X-goog-api-key header")
		}

		// Verify request body structure
		var req googleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		if len(req.Contents) == 0 {
			t.Error("Expected content in request")
		}

		if len(req.Contents[0].Parts) == 0 {
			t.Error("Expected parts in content")
		}

		if req.Contents[0].Parts[0].Text == "" {
			t.Error("Expected text in first part")
		}

		// Return a successful response
		response := googleResponse{
			Candidates: []googleCandidate{
				{
					Content: googleContent{
						Parts: []googlePart{
							{
								Text: "This is a response from Gemini",
							},
						},
					},
					FinishReason: "STOP",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := GeminiConfig{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}
	provider := NewGeminiProvider(config)

	ctx := context.Background()
	response, err := provider.GenerateResponse(ctx, "Hello, Gemini!", []Tool{})

	if err != nil {
		t.Fatalf("GenerateResponse failed: %v", err)
	}

	if response.Content != "This is a response from Gemini" {
		t.Errorf("Expected specific content, got '%s'", response.Content)
	}

	if !response.Finished {
		t.Error("Response should be marked as finished")
	}

	if len(response.ToolCalls) != 0 {
		t.Error("Should have no tool calls")
	}
}

func TestGeminiProvider_GenerateResponse_WithToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify tools are included in request
		var req googleRequest
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.Tools) == 0 {
			t.Error("Expected tools in request")
		}

		if len(req.Tools[0].FunctionDeclarations) == 0 {
			t.Error("Expected function declarations")
		}

		// Return response with function call
		response := googleResponse{
			Candidates: []googleCandidate{
				{
					Content: googleContent{
						Parts: []googlePart{
							{
								Text: "I'll help you read that file.",
							},
							{
								FunctionCall: &googleFunctionCall{
									Name: "read_file",
									Args: map[string]interface{}{
										"path": "test.txt",
									},
								},
							},
						},
					},
					FinishReason: "STOP",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := GeminiConfig{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}
	provider := NewGeminiProvider(config)

	tools := []Tool{
		{
			Name:        "read_file",
			Description: "Read a file",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
	}

	ctx := context.Background()
	response, err := provider.GenerateResponse(ctx, "Read test.txt", tools)

	if err != nil {
		t.Fatalf("GenerateResponse failed: %v", err)
	}

	if response.Content != "I'll help you read that file." {
		t.Errorf("Expected specific content, got '%s'", response.Content)
	}

	if len(response.ToolCalls) != 1 {
		t.Errorf("Expected 1 tool call, got %d", len(response.ToolCalls))
	}

	if response.ToolCalls[0].Name != "read_file" {
		t.Errorf("Expected tool call 'read_file', got '%s'", response.ToolCalls[0].Name)
	}

	// Verify tool call arguments
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(response.ToolCalls[0].Arguments), &args); err != nil {
		t.Fatalf("Failed to parse tool call arguments: %v", err)
	}

	if args["path"] != "test.txt" {
		t.Errorf("Expected path 'test.txt', got '%v'", args["path"])
	}
}

func TestGeminiProvider_GenerateResponse_ErrorResponses(t *testing.T) {
	tests := []struct {
		name            string
		finishReason    string
		expectedContent string
		shouldFinish    bool
	}{
		{
			name:            "malformed function call",
			finishReason:    "MALFORMED_FUNCTION_CALL",
			expectedContent: "",    // No error message set anymore
			shouldFinish:    false, // Don't mark as finished - let tool results determine outcome
		},
		{
			name:            "max tokens",
			finishReason:    "MAX_TOKENS",
			expectedContent: "I reached the maximum response length",
			shouldFinish:    true,
		},
		{
			name:            "safety filter",
			finishReason:    "SAFETY",
			expectedContent: "My response was filtered due to safety policies",
			shouldFinish:    true,
		},
		{
			name:            "recitation filter",
			finishReason:    "RECITATION",
			expectedContent: "My response was filtered due to potential copyright issues",
			shouldFinish:    true,
		},
		{
			name:            "empty finish reason",
			finishReason:    "",
			expectedContent: "",
			shouldFinish:    false,
		},
		{
			name:            "unknown finish reason",
			finishReason:    "UNKNOWN_REASON",
			expectedContent: "",
			shouldFinish:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				response := googleResponse{
					Candidates: []googleCandidate{
						{
							Content: googleContent{
								Parts: []googlePart{
									{
										Text: "Original response text",
									},
								},
							},
							FinishReason: tt.finishReason,
						},
					},
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			config := GeminiConfig{
				APIKey:   "test-key",
				Endpoint: server.URL,
			}
			provider := NewGeminiProvider(config)

			ctx := context.Background()
			response, err := provider.GenerateResponse(ctx, "Test prompt", []Tool{})

			if err != nil {
				t.Fatalf("GenerateResponse failed: %v", err)
			}

			if tt.expectedContent != "" {
				if !strings.Contains(response.Content, tt.expectedContent) {
					t.Errorf("Expected content to contain '%s', got '%s'", tt.expectedContent, response.Content)
				}
			} else if tt.finishReason == "" {
				// For empty finish reason, should preserve original text
				if response.Content != "Original response text" {
					t.Errorf("Expected original text, got '%s'", response.Content)
				}
			}

			if response.Finished != tt.shouldFinish {
				t.Errorf("Expected finished=%t, got %t", tt.shouldFinish, response.Finished)
			}
		})
	}
}

func TestGeminiProvider_GenerateResponse_HTTPErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantError  bool
	}{
		{
			name:       "success",
			statusCode: http.StatusOK,
			wantError:  false,
		},
		{
			name:       "bad request",
			statusCode: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "unauthorized",
			statusCode: http.StatusUnauthorized,
			wantError:  true,
		},
		{
			name:       "rate limited",
			statusCode: http.StatusTooManyRequests,
			wantError:  true,
		},
		{
			name:       "server error",
			statusCode: http.StatusInternalServerError,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.statusCode == http.StatusOK {
					response := googleResponse{
						Candidates: []googleCandidate{
							{
								Content: googleContent{
									Parts: []googlePart{
										{Text: "Success response"},
									},
								},
								FinishReason: "STOP",
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(response)
				} else {
					w.WriteHeader(tt.statusCode)
					w.Write([]byte("Error response"))
				}
			}))
			defer server.Close()

			config := GeminiConfig{
				APIKey:   "test-key",
				Endpoint: server.URL,
			}
			provider := NewGeminiProvider(config)

			ctx := context.Background()
			response, err := provider.GenerateResponse(ctx, "Test", []Tool{})

			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				if !strings.Contains(err.Error(), "gemini API returned status") {
					t.Errorf("Expected API status error, got: %v", err)
				}
				if response != nil {
					t.Error("Should not return response on error")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if response == nil {
					t.Error("Expected response")
				}
			}
		})
	}
}

func TestGeminiProvider_GenerateResponse_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json response"))
	}))
	defer server.Close()

	config := GeminiConfig{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}
	provider := NewGeminiProvider(config)

	ctx := context.Background()
	response, err := provider.GenerateResponse(ctx, "Test", []Tool{})

	if err == nil {
		t.Error("Expected error for invalid JSON")
	}

	if !strings.Contains(err.Error(), "failed to decode response") {
		t.Errorf("Expected decode error, got: %v", err)
	}

	if response != nil {
		t.Error("Should not return response on JSON error")
	}
}

func TestGeminiProvider_GenerateResponse_NetworkError(t *testing.T) {
	// Use an invalid endpoint to simulate network error
	config := GeminiConfig{
		APIKey:   "test-key",
		Endpoint: "http://invalid-endpoint-that-does-not-exist.com",
	}
	provider := NewGeminiProvider(config)

	ctx := context.Background()
	response, err := provider.GenerateResponse(ctx, "Test", []Tool{})

	if err == nil {
		t.Error("Expected network error")
	}

	if !strings.Contains(err.Error(), "failed to send request") {
		t.Errorf("Expected network error, got: %v", err)
	}

	if response != nil {
		t.Error("Should not return response on network error")
	}
}

func TestGeminiProvider_GenerateResponse_ContextCancellation(t *testing.T) {
	// Create a server with a delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(100 * time.Millisecond)

		response := googleResponse{
			Candidates: []googleCandidate{
				{
					Content: googleContent{
						Parts: []googlePart{
							{Text: "Delayed response"},
						},
					},
					FinishReason: "STOP",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := GeminiConfig{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}
	provider := NewGeminiProvider(config)

	// Create a context that cancels quickly
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	response, err := provider.GenerateResponse(ctx, "Test", []Tool{})

	// Should either timeout or succeed depending on timing
	if err != nil {
		if !strings.Contains(err.Error(), "context") && !strings.Contains(err.Error(), "timeout") {
			t.Errorf("Expected context/timeout error, got: %v", err)
		}
	}

	// If no error, response should be valid
	if err == nil && response == nil {
		t.Error("If no error, should have response")
	}
}

func TestGeminiProvider_RequestFormat(t *testing.T) {
	// Test that the request format matches Gemini API expectations
	var capturedRequest googleRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture the request for validation
		if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
			t.Errorf("Failed to decode request: %v", err)
		}

		// Return minimal response
		response := googleResponse{
			Candidates: []googleCandidate{
				{
					Content: googleContent{
						Parts: []googlePart{{Text: "OK"}},
					},
					FinishReason: "STOP",
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := GeminiConfig{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}
	provider := NewGeminiProvider(config)

	tools := []Tool{
		{
			Name:        "test_tool",
			Description: "A test tool",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"param": map[string]interface{}{"type": "string"},
				},
			},
		},
	}

	ctx := context.Background()
	_, err := provider.GenerateResponse(ctx, "Test prompt", tools)

	if err != nil {
		t.Fatalf("GenerateResponse failed: %v", err)
	}

	// Validate request structure
	if len(capturedRequest.Contents) != 1 {
		t.Errorf("Expected 1 content, got %d", len(capturedRequest.Contents))
	}

	if len(capturedRequest.Contents[0].Parts) != 1 {
		t.Errorf("Expected 1 part, got %d", len(capturedRequest.Contents[0].Parts))
	}

	if capturedRequest.Contents[0].Parts[0].Text != "Test prompt" {
		t.Errorf("Expected prompt in text, got '%s'", capturedRequest.Contents[0].Parts[0].Text)
	}

	if len(capturedRequest.Tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(capturedRequest.Tools))
	}

	if len(capturedRequest.Tools[0].FunctionDeclarations) != 1 {
		t.Errorf("Expected 1 function declaration, got %d", len(capturedRequest.Tools[0].FunctionDeclarations))
	}

	funcDecl := capturedRequest.Tools[0].FunctionDeclarations[0]
	if funcDecl.Name != "test_tool" {
		t.Errorf("Expected function name 'test_tool', got '%s'", funcDecl.Name)
	}

	if funcDecl.Description != "A test tool" {
		t.Errorf("Expected function description 'A test tool', got '%s'", funcDecl.Description)
	}

	if capturedRequest.GenerationConfig == nil {
		t.Error("Expected generation config")
	}

	if capturedRequest.GenerationConfig.MaxOutputTokens != 4096 {
		t.Errorf("Expected max tokens 4096, got %d", capturedRequest.GenerationConfig.MaxOutputTokens)
	}
}

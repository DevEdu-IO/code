// Package client is a thin HTTP client for the DevEdu API. DevEdu authenticates
// the user by their API key and proxies the prompt to Amazon Bedrock.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNoAgent means the DevEdu instance has no Bedrock Agent configured, so the
// caller should fall back to the plain /chat endpoint.
var ErrNoAgent = errors.New("no agent configured")

// Client talks to a DevEdu instance's /api/v1 endpoints.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// New builds a client for the given DevEdu base URL and API key.
func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}
}

type chatRequest struct {
	Prompt string `json:"prompt"`
}

type chatResponse struct {
	Response string `json:"response"`
	Error    string `json:"error"`
}

// Chat sends a prompt and returns the model's reply. Errors are normalized into
// friendly messages (bad key, upstream failure, network).
func (c *Client) Chat(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(chatRequest{Prompt: prompt})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not reach DevEdu at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var parsed chatResponse
	_ = json.Unmarshal(data, &parsed)

	switch resp.StatusCode {
	case http.StatusOK:
		return parsed.Response, nil
	case http.StatusUnauthorized:
		return "", fmt.Errorf("unauthorized - check your API key (DEVEDU_API_KEY)")
	case http.StatusBadGateway:
		return "", fmt.Errorf("the AI service is unavailable right now, please try again")
	default:
		msg := parsed.Error
		if msg == "" {
			msg = strings.TrimSpace(string(data))
		}
		return "", fmt.Errorf("DevEdu returned %d: %s", resp.StatusCode, msg)
	}
}

// ---- agent (tool-using) endpoint ----------------------------------------

// ToolCall is a tool the agent wants the CLI to run.
type ToolCall struct {
	ActionGroup string            `json:"action_group"`
	Function    string            `json:"function"`
	Params      map[string]string `json:"params"`
}

// ToolResult is the outcome of a tool the CLI ran, sent back to resume the agent.
type ToolResult struct {
	ActionGroup string `json:"action_group"`
	Function    string `json:"function"`
	Output      string `json:"output"`
}

// AgentResponse is one step of the agent loop. When Done is true, Text is the
// final answer; otherwise ToolCalls must be executed and posted back.
type AgentResponse struct {
	SessionID    string     `json:"session_id"`
	Text         string     `json:"text"`
	InvocationID string     `json:"invocation_id"`
	Done         bool       `json:"done"`
	ToolCalls    []ToolCall `json:"tool_calls"`
	ErrorCode    string     `json:"error"`
}

// AgentMessage starts/continues an agent conversation with a user message.
func (c *Client) AgentMessage(ctx context.Context, sessionID, message string) (*AgentResponse, error) {
	return c.agent(ctx, map[string]any{"session_id": sessionID, "message": message})
}

// AgentToolResults resumes a turn after the CLI executed the agent's tool calls.
func (c *Client) AgentToolResults(ctx context.Context, sessionID, invocationID string, results []ToolResult) (*AgentResponse, error) {
	return c.agent(ctx, map[string]any{"session_id": sessionID, "invocation_id": invocationID, "tool_results": results})
}

func (c *Client) agent(ctx context.Context, body map[string]any) (*AgentResponse, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/agent", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach DevEdu at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var parsed AgentResponse
	_ = json.Unmarshal(raw, &parsed)

	switch resp.StatusCode {
	case http.StatusOK:
		return &parsed, nil
	case http.StatusConflict: // no agent configured on this instance
		return nil, ErrNoAgent
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("unauthorized - check your API key (DEVEDU_API_KEY)")
	case http.StatusBadGateway:
		return nil, fmt.Errorf("the AI service is unavailable right now, please try again")
	default:
		msg := parsed.ErrorCode
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return nil, fmt.Errorf("DevEdu returned %d: %s", resp.StatusCode, msg)
	}
}

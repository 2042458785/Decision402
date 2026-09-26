package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Reasoning  string     `json:"reasoning_content,omitempty"`
}
type Model struct {
	Key, Name, BaseURL string
	HTTP               *http.Client
}

func NewModel(key, name, base string) *Model {
	return &Model{Key: key, Name: name, BaseURL: base, HTTP: &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func tool(name, description string, properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{"name": name, "description": description, "parameters": map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}}}
}

var agentTools = []map[string]any{
	tool("find_services", "Find and screen paid services for a Tokyo weather dataset. Only Tokyo is supported. Does not pay.", map[string]any{"city": map[string]any{"type": "string", "description": "City requested by the user. Only Tokyo is available."}}, []string{"city"}),
	tool("execute_purchase", "After find_services, ask the server to select under the user's immutable policy and execute this task once. Cannot override policy or choose a payee. Simulation never pays.", map[string]any{}, []string{}),
}

func (m *Model) Complete(ctx context.Context, messages []Message) (Message, error) {
	body, _ := json.Marshal(map[string]any{"model": m.Name, "messages": messages, "tools": agentTools, "max_tokens": 900, "temperature": 0, "thinking": map[string]string{"type": "disabled"}})
	req, err := http.NewRequestWithContext(ctx, "POST", m.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Message{}, errors.New("Could not construct the model request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.Key)
	resp, err := m.HTTP.Do(req)
	if err != nil {
		return Message{}, errors.New("Model request failed or timed out; no automatic retry")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return Message{}, fmt.Errorf("DeepSeek HTTP %d; check the API key, credits, and model configuration", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Message{}, errors.New("Could not read the model response")
	}
	var result struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(b, &result) != nil || len(result.Choices) != 1 {
		return Message{}, errors.New("Model response format is invalid")
	}
	return result.Choices[0].Message, nil
}

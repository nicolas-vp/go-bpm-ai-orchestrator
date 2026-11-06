// Package ollama подключает две роли AI task к локальным моделям Ollama.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nicolas-vp/go-bpm-ai-orchestrator/engine"
)

type Client struct {
	URL      string
	Writer   string
	Reviewer string
	HTTP     *http.Client
}

func (c *Client) Complete(ctx context.Context, request engine.AIRequest) (string, error) {
	model := c.Writer
	switch request.Model {
	case "writer":
	case "reviewer":
		model = c.Reviewer
	default:
		return "", fmt.Errorf("unknown AI role %q", request.Model)
	}
	if model == "" {
		return "", fmt.Errorf("model for role %q is required", request.Model)
	}
	payload := map[string]any{
		"model": model, "stream": false, "keep_alive": 0,
		"messages": []map[string]string{{"role": "system", "content": request.SystemPrompt}, {"role": "user", "content": request.Prompt}},
	}
	if request.JSON {
		payload["format"] = map[string]any{"type": "object", "properties": map[string]any{"approved": map[string]string{"type": "boolean"}, "feedback": map[string]string{"type": "string"}}, "required": []string{"approved", "feedback"}, "additionalProperties": false}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.URL, "/")+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	response, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama %s: %w", request.Model, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama HTTP %d: %.500s", response.StatusCode, data)
	}
	var answer struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Done       bool   `json:"done"`
		DoneReason string `json:"done_reason"`
		Error      string `json:"error"`
	}
	if err := json.Unmarshal(data, &answer); err != nil {
		return "", fmt.Errorf("ollama response: %w", err)
	}
	if answer.Error != "" {
		return "", fmt.Errorf("ollama: %s", answer.Error)
	}
	if !answer.Done || answer.DoneReason == "length" || strings.TrimSpace(answer.Message.Content) == "" {
		return "", fmt.Errorf("ollama returned incomplete or empty response")
	}
	if request.JSON {
		if err := validateReview(answer.Message.Content); err != nil {
			return "", err
		}
	}
	return answer.Message.Content, nil
}

func validateReview(content string) error {
	var result struct {
		Approved *bool  `json:"approved"`
		Feedback string `json:"feedback"`
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return fmt.Errorf("invalid review: %w", err)
	}
	if result.Approved == nil || strings.TrimSpace(result.Feedback) == "" {
		return fmt.Errorf("review requires approved boolean and nonempty feedback")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("review has trailing data")
	}
	return nil
}

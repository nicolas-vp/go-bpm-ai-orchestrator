package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nicolas-vp/go-bpm-ai-orchestrator/engine"
)

func TestReviewContract(t *testing.T) {
	for _, content := range []string{`{}`, `{"approved":"true","feedback":"ok"}`, `{"approved":true}`, `{"approved":true,"feedback":" "}`, `{"approved":true,"feedback":"ok","extra":1}`, `{"approved":true,"feedback":"ok"} {}`} {
		if err := validateReview(content); err == nil {
			t.Errorf("accepted invalid review: %s", content)
		}
	}
	for _, content := range []string{`{"approved":true,"feedback":"ok"}`, `{"approved":false,"feedback":"fix nil case"}`} {
		if err := validateReview(content); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRolesAndFailures(t *testing.T) {
	tests := []struct {
		name, role, content string
		status              int
		done                bool
		reason              string
		wantErr             bool
	}{
		{"writer", "writer", "solution", 200, true, "stop", false},
		{"reviewer", "reviewer", `{"approved":false,"feedback":"fix"}`, 200, true, "stop", false},
		{"incomplete", "writer", "partial", 200, false, "", true},
		{"truncated", "writer", "partial", 200, true, "length", true},
		{"missing verdict", "reviewer", `{"feedback":"fix"}`, 200, true, "stop", true},
		{"server failure", "writer", "", 503, false, "", true},
		{"unknown role", "other", "", 200, true, "stop", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				// Параметры оптимизированной модели наследуем из Modelfile.
				if r.URL.Path != "/api/chat" {
					t.Errorf("path %s", r.URL.Path)
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if payload["options"] != nil {
					t.Error("model parameters were overridden")
				}
				want := "qwen-writer"
				if tt.role == "reviewer" {
					want = "qwen-reviewer"
					if payload["format"] == nil {
						t.Error("missing JSON schema")
					}
				}
				if payload["model"] != want || payload["stream"] != false || payload["keep_alive"] != float64(0) {
					t.Errorf("payload %v", payload)
				}
				w.WriteHeader(tt.status)
				_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": tt.content}, "done": tt.done, "done_reason": tt.reason})
			}))
			defer server.Close()
			c := &Client{URL: server.URL, Writer: "qwen-writer", Reviewer: "qwen-reviewer", HTTP: server.Client()}
			_, err := c.Complete(context.Background(), engine.AIRequest{Model: tt.role, JSON: tt.role == "reviewer"})
			if (err != nil) != tt.wantErr {
				t.Fatalf("error %v, wantErr %v", err, tt.wantErr)
			}
			if tt.role == "other" && called {
				t.Error("unknown role reached server")
			}
		})
	}
}

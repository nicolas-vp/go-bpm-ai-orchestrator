package scenarios_test

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicolas-vp/go-bpm-ai-orchestrator/engine"
	"github.com/nicolas-vp/go-bpm-ai-orchestrator/internal/bpmgen"
	"github.com/nicolas-vp/go-bpm-ai-orchestrator/internal/bpmgen/services"
	"github.com/nicolas-vp/go-bpm-ai-orchestrator/internal/pgtest"
	"github.com/nicolas-vp/go-bpm-ai-orchestrator/queue"
)

// Проверяем с настоящей очередью и сгенерированным BPMN; ответы моделей детерминированы.
func TestOrchestration(t *testing.T) {
	for _, tc := range []struct {
		name           string
		approveAt, max int
		outcome        string
		rounds         int
	}{
		{"accept first", 1, 3, "accepted", 1},
		{"revise and accept", 2, 3, "accepted", 2},
		{"limit", 99, 2, "needs_human", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := pgtest.Pool(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			log := slog.New(slog.DiscardHandler)
			svc := &services.Services{}
			processes := bpmgen.Processes(svc)
			var mu sync.Mutex
			var calls []string
			writes, reviews := 0, 0
			client := engine.AIClientFunc(func(ctx context.Context, r engine.AIRequest) (string, error) {
				mu.Lock()
				defer mu.Unlock()
				calls = append(calls, r.Model)
				if r.Model == "writer" {
					writes++
					if writes > 1 && !strings.Contains(r.Prompt, "fix edge case") {
						return "", fmt.Errorf("feedback lost")
					}
					return fmt.Sprintf("draft %d", writes), nil
				}
				reviews++
				if !strings.Contains(r.Prompt, fmt.Sprintf("draft %d", writes)) {
					return "", fmt.Errorf("draft lost")
				}
				return fmt.Sprintf(`{"approved":%t,"feedback":"fix edge case"}`, reviews >= tc.approveAt), nil
			})
			q := queue.New(pool, queue.Options{Logger: log, PollInterval: 10 * time.Millisecond})
			e, err := engine.New(q, engine.Config{Logger: log, AI: client, History: engine.HistoryConfig{ToDB: func() bool { return true }}}, processes, bpmgen.Queues(svc))
			if err != nil {
				t.Fatal(err)
			}
			reader := engine.NewHistoryReader(pool, processes, nil)
			q.Start(ctx)
			defer q.Stop()
			id, err := e.StartProcess(ctx, "AiOrchestrator", map[string]any{"goal": "test task", "maxRounds": tc.max})
			if err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(10 * time.Second)
			for {
				instance, err := reader.GetInstance(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				if instance.Status == engine.InstanceCompleted {
					break
				}
				if instance.Status == engine.InstanceFailed || time.Now().After(deadline) {
					t.Fatalf("instance: %+v", instance)
				}
				time.Sleep(20 * time.Millisecond)
			}
			history, err := reader.GetHistory(ctx, id, false)
			if err != nil {
				t.Fatal(err)
			}
			var vars map[string]any
			for _, h := range history {
				if h.Variables != nil {
					vars = h.Variables
				}
			}
			if vars["outcome"] != tc.outcome || vars["round"] != float64(tc.rounds) {
				t.Fatalf("variables: %v", vars)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(calls) != tc.rounds*2 {
				t.Fatalf("calls: %v", calls)
			}
			for i, role := range calls {
				want := "writer"
				if i%2 == 1 {
					want = "reviewer"
				}
				if role != want {
					t.Fatalf("calls: %v", calls)
				}
			}
		})
	}
}

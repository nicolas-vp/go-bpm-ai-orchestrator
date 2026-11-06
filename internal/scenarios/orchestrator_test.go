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

// Настоящая очередь и сгенерированный BPMN, ответы моделей детерминированы.
func TestOrchestration(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		approveAt, max, rounds      int
		outcome, human, timeoutRole string
	}{
		{name: "accept first", approveAt: 1, max: 3, rounds: 1, outcome: "accepted"},
		{name: "revise and accept", approveAt: 2, max: 3, rounds: 2, outcome: "accepted"},
		{name: "limit human approves", approveAt: 99, max: 2, rounds: 2, outcome: "approved_by_human", human: "approve"},
		{name: "limit human rejects", approveAt: 99, max: 2, rounds: 2, outcome: "rejected_by_human", human: "reject"},
		{name: "writer timeout", approveAt: 1, max: 3, rounds: 1, outcome: "rejected_by_human", human: "reject", timeoutRole: "writer"},
		{name: "reviewer timeout", approveAt: 1, max: 3, rounds: 1, outcome: "approved_by_human", human: "approve", timeoutRole: "reviewer"},
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
			release := make(chan struct{})
			lateDone := make(chan struct{})
			var releaseOnce sync.Once
			releaseModel := func() { releaseOnce.Do(func() { close(release) }) }
			defer releaseModel()
			client := engine.AIClientFunc(func(ctx context.Context, r engine.AIRequest) (string, error) {
				mu.Lock()
				calls = append(calls, r.Model)
				var answer string
				var err error
				if r.Model == "writer" {
					writes++
					if writes > 1 && !strings.Contains(r.Prompt, "fix edge case") {
						err = fmt.Errorf("feedback lost")
					}
					answer = fmt.Sprintf("draft %d", writes)
				} else {
					reviews++
					if !strings.Contains(r.Prompt, fmt.Sprintf("draft %d", writes)) {
						err = fmt.Errorf("draft lost")
					}
					answer = fmt.Sprintf(`{"approved":%t,"feedback":"fix edge case"}`, reviews >= tc.approveAt)
				}
				mu.Unlock()
				if r.Model == tc.timeoutRole {
					// Имитируем провайдера, который ответил уже после таймера и ручного решения.
					select {
					case <-release:
					case <-ctx.Done():
						return "", ctx.Err()
					}
					defer close(lateDone)
				}
				return answer, err
			})
			q := queue.New(pool, queue.Options{Logger: log, PollInterval: 10 * time.Millisecond})
			e, err := engine.New(q, engine.Config{Logger: log, AI: client, History: engine.HistoryConfig{ToDB: func() bool { return true }}}, processes, bpmgen.Queues(svc))
			if err != nil {
				t.Fatal(err)
			}
			reader := engine.NewHistoryReader(pool, processes, nil)
			tasks := engine.NewUserTaskService(pool, e)
			q.Start(ctx)
			defer q.Stop()
			// При ранней ошибке разблокировать клиент нужно до q.Stop.
			defer releaseModel()
			variables := map[string]any{"goal": "test task", "maxRounds": tc.max}
			if tc.timeoutRole != "" {
				variables["aiTimeoutSeconds"] = 1
			}
			id, err := e.StartProcess(ctx, "AiOrchestrator", variables)
			if err != nil {
				t.Fatal(err)
			}
			if tc.human != "" {
				deadline := time.Now().Add(10 * time.Second)
				var task engine.UserTask
				for {
					list, err := tasks.ListTasks(ctx, engine.TaskFilter{InstanceUid: id})
					if err != nil {
						t.Fatal(err)
					}
					if len(list) == 1 {
						task = list[0]
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("human task not created")
					}
					time.Sleep(20 * time.Millisecond)
				}
				if task.ElementId != "HumanReview" {
					t.Fatalf("task: %+v", task)
				}
				instance, err := reader.GetInstance(ctx, id)
				if err != nil || instance.Status != engine.InstanceRunning {
					t.Fatalf("instance must wait for human: %+v, %v", instance, err)
				}
				detail, err := tasks.GetTask(ctx, task.TaskUid)
				if err != nil {
					t.Fatal(err)
				}
				reason := "iteration_limit"
				if tc.timeoutRole != "" {
					reason = "ai_timeout"
				}
				if detail.Variables["outcome"] != "needs_human" || detail.Variables["escalationReason"] != reason {
					t.Fatalf("human variables: %v", detail.Variables)
				}
				// Проверяем отсутствие автоматического завершения, пока человек не ответил.
				time.Sleep(50 * time.Millisecond)
				if err := tasks.Claim(ctx, task.TaskUid, "reviewer", []string{"reviewers"}); err != nil {
					t.Fatal(err)
				}
				if err := tasks.Complete(ctx, task.TaskUid, "reviewer", map[string]any{"humanApproved": tc.human == "approve", "humanComment": "checked manually"}); err != nil {
					t.Fatal(err)
				}
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
			if tc.timeoutRole != "" {
				releaseModel()
				select {
				case <-lateDone:
				case <-time.After(5 * time.Second):
					t.Fatal("late model did not finish")
				}
				// Stop ждёт и транзакцию уже исполняемого обработчика.
				q.Stop()
			}
			var pending int
			if err := pool.QueryRow(ctx, `select count(*) from bpm_queue where status='pending'`).Scan(&pending); err != nil {
				t.Fatal(err)
			}
			if pending != 0 {
				t.Fatalf("pending tasks after completion: %d", pending)
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
			if tc.human != "" && vars["humanComment"] != "checked manually" {
				t.Fatalf("comment lost: %v", vars)
			}
			if tc.timeoutRole != "" {
				boundary := "WriteTimeout"
				timedOut := "Write"
				if tc.timeoutRole == "reviewer" {
					boundary = "ReviewTimeout"
					timedOut = "Review"
				}
				fired := false
				for _, h := range history {
					if h.ElementId == boundary && h.Event == engine.HistoryCompleted {
						fired = true
					}
					if h.ElementId == timedOut && h.Event == engine.HistoryCompleted {
						t.Fatal("late response completed interrupted task")
					}
				}
				if !fired {
					t.Fatal("boundary timer missing in history")
				}
			}
			mu.Lock()
			defer mu.Unlock()
			wantCalls := tc.rounds * 2
			if tc.timeoutRole == "writer" {
				wantCalls = 1
			}
			if len(calls) != wantCalls {
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

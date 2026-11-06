package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/nicolas-vp/go-bpm-ai-orchestrator/bpm"

	"github.com/nicolas-vp/go-bpm-ai-orchestrator/queue"
)

type fakeQueue struct {
	pushed []queue.Task
	// cancelled - ключи отменённых задач "<kind>/<key>"
	cancelled []string
}

func (q *fakeQueue) Register(string, queue.Handler, queue.KindOptions) {}

func (q *fakeQueue) Push(_ context.Context, task queue.Task, _ ...queue.PushOption) error {
	q.pushed = append(q.pushed, task)
	return nil
}

// InTx вызывает fn с транзакцией, которая принимает любые запросы: экземпляр, который пишет StartProcess
func (q *fakeQueue) InTx(_ context.Context, fn func(tx queue.Tx) error) error {
	return fn(fakeExecTx{})
}

type fakeExecTx struct{ queue.Tx }

func (fakeExecTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (q *fakeQueue) PushTx(ctx context.Context, _ queue.Tx, task queue.Task, opts ...queue.PushOption) error {
	return q.Push(ctx, task, opts...)
}

// CancelByKeyTx убирает из очереди задачи с ключом, как очередь отменяет ещё не завершённые
func (q *fakeQueue) CancelByKeyTx(_ context.Context, _ queue.Tx, kind, key string) error {
	q.cancelled = append(q.cancelled, fmt.Sprintf("%s/%s", kind, key))
	q.pushed = slices.DeleteFunc(q.pushed, func(task queue.Task) bool {
		return task.Kind == kind && task.Key == key
	})
	return nil
}

// RetryFailedTx - упавших задач в фейковой очереди нет
func (q *fakeQueue) RetryFailedTx(_ context.Context, _ queue.Tx, id int64, _ []byte) error {
	return fmt.Errorf("task %d: %w", id, queue.ErrNotFailed)
}

// popKind достаёт первую задачу вида kind: порядок задач разных видов в очереди не гарантирован
func (q *fakeQueue) popKind(t *testing.T, kind string) queue.Task {
	t.Helper()
	i := slices.IndexFunc(q.pushed, func(task queue.Task) bool { return task.Kind == kind })
	if i < 0 {
		t.Fatalf("no task of kind %s in queue", kind)
	}
	task := q.pushed[i]
	q.pushed = slices.Delete(q.pushed, i, i+1)
	return task
}

func (q *fakeQueue) pop(t *testing.T) queue.Task {
	t.Helper()
	if len(q.pushed) == 0 {
		t.Fatal("queue is empty")
	}
	task := q.pushed[0]
	q.pushed = q.pushed[1:]
	return task
}

// fakeSyncTx эмулирует таблицу bpm_parallel_sync.
type fakeSyncTx struct {
	queue.Tx
	arrived   map[string]int
	variables map[string]map[string]any
}

type fakeRow struct {
	arrived   int
	variables []byte
}

func (r fakeRow) Scan(dest ...any) error {
	*dest[0].(*int) = r.arrived
	*dest[1].(*[]byte) = r.variables
	return nil
}

func (tx *fakeSyncTx) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	hash := args[0].(string)
	var incoming map[string]any
	_ = json.Unmarshal([]byte(args[1].(string)), &incoming)
	if tx.variables[hash] == nil {
		tx.variables[hash] = map[string]any{}
	}
	for k, v := range incoming {
		tx.variables[hash][k] = v
	}
	tx.arrived[hash]++
	payload, _ := json.Marshal(tx.variables[hash])
	return fakeRow{arrived: tx.arrived[hash], variables: payload}
}

func (tx *fakeSyncTx) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	delete(tx.arrived, args[0].(string))
	return pgconn.CommandTag{}, nil
}

// generatedProcesses - процессы из сгенерированного кода с пустыми сервисами

func newTestService(t *testing.T, deployment map[string]bpm.DeploymentProcess, queues []bpm.Queue) (*Engine, *fakeQueue) {
	t.Helper()
	return newTestServiceFromProcesses(t, map[string]bpm.Process{"test": {Flows: deployment}}, queues)
}

func newTestServiceFromProcesses(t *testing.T, processes map[string]bpm.Process, queues []bpm.Queue) (*Engine, *fakeQueue) {
	t.Helper()
	queue := &fakeQueue{}
	service, err := New(queue, Config{Logger: slog.New(slog.DiscardHandler)}, processes, queues)
	if err != nil {
		t.Fatal(err)
	}
	return service, queue
}

func startAndActivate(t *testing.T, s *Engine, q *fakeQueue, tx queue.Tx, processName string, variables map[string]any, opts ...StartOption) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.StartProcess(ctx, processName, variables, opts...); err != nil {
		t.Fatal(err)
	}
	var st systemTask
	if err := json.Unmarshal(q.pop(t).Payload, &st); err != nil {
		t.Fatal(err)
	}
	if err := s.startRoot(ctx, tx, st.Message); err != nil {
		t.Fatal(err)
	}
}

func completeTask(t *testing.T, s *Engine, q *fakeQueue, tx queue.Tx, wantType string) *instanceMessage {
	t.Helper()
	return completeTaskWith(t, s, q, tx, wantType, nil)
}

// completeTaskWith выполняет задачу wantType; handle, если задан, играет роль обработчика и может менять сообщение
func completeTaskWith(t *testing.T, s *Engine, q *fakeQueue, tx queue.Tx, wantType string, handle func(*instanceMessage)) *instanceMessage {
	t.Helper()
	task := q.pop(t)
	if task.Kind != s.taskKinds[wantType] {
		t.Fatalf("got task kind %s, want %q (%s)", task.Kind, wantType, s.taskKinds[wantType])
	}
	var message instanceMessage
	if err := json.Unmarshal(task.Payload, &message); err != nil {
		t.Fatal(err)
	}
	if handle != nil {
		handle(&message)
	}
	if err := s.moveNext(context.Background(), tx, &message, message.CurrentPosition, moveComplete); err != nil {
		t.Fatal(err)
	}
	return &message
}

// TestGeneratedProcesses проверяет, что движок принимает все сгенерированные процессы

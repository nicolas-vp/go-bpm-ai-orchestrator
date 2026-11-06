package engine_test

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nicolas-vp/go-bpm-ai-orchestrator/engine"
	"testing"
	"time"
)

type messageTest struct {
	t      *testing.T
	pool   *pgxpool.Pool
	e      *engine.Engine
	reader *engine.HistoryReader
}

func (mt *messageTest) waitStatus(id uuid.UUID, status string) map[string]any {
	mt.t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(5 * time.Second)
	for {
		instance, err := mt.reader.GetInstance(ctx, id)
		if err == nil && instance.Status == status {
			break
		}
		if time.Now().After(deadline) {
			mt.t.Fatalf("instance %s: %+v, err %v, want %s", id, instance, err, status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	history, err := mt.reader.GetHistory(ctx, id, false)
	if err != nil {
		mt.t.Fatal(err)
	}
	var variables map[string]any
	for _, record := range history {
		if record.Variables != nil {
			variables = record.Variables
		}
	}
	return variables
}

func waitQueue(t *testing.T, pool *pgxpool.Pool, timeout time.Duration) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	for {
		var pending, failed int
		err := pool.QueryRow(ctx, `select count(*) filter (where status = 'pending'), count(*) filter (where status = 'failed') from bpm_queue`).Scan(&pending, &failed)
		if err != nil {
			t.Fatal(err)
		}
		if failed > 0 {
			var lastError string
			_ = pool.QueryRow(ctx, `select kind || ': ' || coalesce(last_error, '') from bpm_queue where status = 'failed' limit 1`).Scan(&lastError)
			t.Fatalf("%d failed tasks, first: %s", failed, lastError)
		}
		if pending == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d tasks still pending", pending)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

package ai_orchestrator

import (
	"context"
	"fmt"
	"github.com/nicolas-vp/go-bpm-ai-orchestrator/internal/bpmgen/model"
	"github.com/nicolas-vp/go-bpm-ai-orchestrator/internal/bpmgen/services"
	"strings"
	"time"
)

func Prepare(ctx context.Context, svc *services.Services, data *model.DataModel) error {
	if data.MaxRounds == 0 {
		data.MaxRounds = 3
	}
	if data.MaxRounds < 1 || data.MaxRounds > 5 {
		return fmt.Errorf("maxRounds must be between 1 and 5")
	}
	if strings.TrimSpace(data.Goal) == "" {
		return fmt.Errorf("goal is required")
	}
	if data.AITimeoutSeconds == 0 {
		data.AITimeoutSeconds = 120
	}
	if data.AITimeoutSeconds < 1 || data.AITimeoutSeconds > 600 {
		return fmt.Errorf("aiTimeoutSeconds must be between 1 and 600")
	}
	data.IterationDeadline = time.Now().Add(time.Duration(data.AITimeoutSeconds) * time.Second).UTC().Format(time.RFC3339Nano)
	data.Round++
	return nil
}

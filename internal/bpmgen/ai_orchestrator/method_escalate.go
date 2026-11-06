package ai_orchestrator

import (
	"context"
	"github.com/nicolas-vp/go-bpm-ai-orchestrator/internal/bpmgen/model"
	"github.com/nicolas-vp/go-bpm-ai-orchestrator/internal/bpmgen/services"
)

func Escalate(ctx context.Context, svc *services.Services, data *model.DataModel) error {
	data.Outcome = "needs_human"
	data.EscalationReason = "iteration_limit"
	return nil
}

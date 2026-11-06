package ai_orchestrator

import (
	"context"
	"github.com/nicolas-vp/go-bpm-ai-orchestrator/internal/bpmgen/model"
	"github.com/nicolas-vp/go-bpm-ai-orchestrator/internal/bpmgen/services"
)

func ResolveHuman(ctx context.Context, svc *services.Services, data *model.DataModel) error {
	if data.HumanApproved {
		data.Outcome = "approved_by_human"
	} else {
		data.Outcome = "rejected_by_human"
	}
	return nil
}

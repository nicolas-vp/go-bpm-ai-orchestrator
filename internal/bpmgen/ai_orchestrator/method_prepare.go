package ai_orchestrator

import (
	"context"
	"fmt"
	"github.com/nicolas-vp/go-bpm-ai-orchestrator/internal/bpmgen/model"
	"github.com/nicolas-vp/go-bpm-ai-orchestrator/internal/bpmgen/services"
	"strings"
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
	data.Round++
	return nil
}

// Package orchestration связывает техническую подготовку с переходами BPMN,
// сохраняя диаграмму без вспомогательных задач.
package orchestration

import (
	"context"
	"slices"

	"github.com/nicolas-vp/go-bpm-ai-orchestrator/bpm"
	"github.com/nicolas-vp/go-bpm-ai-orchestrator/internal/bpmgen"
	"github.com/nicolas-vp/go-bpm-ai-orchestrator/internal/bpmgen/model"
	"github.com/nicolas-vp/go-bpm-ai-orchestrator/internal/bpmgen/services"
)

// Processes дополняет сгенерированный граф обработчиками переходов. XML и
// геометрия пользователя остаются источником маршрутов и отображения схемы.
func Processes(svc *services.Services) map[string]bpm.Process {
	processes := bpmgen.Processes(svc)
	for _, graph := range processes["AiOrchestrator"].Flows {
		for id, expression := range graph.Expressions {
			if expression.ActivateType != bpm.ActivateTypeFlow || len(expression.NextElements) != 1 {
				continue
			}
			switch expression.NextElements[0] {
			case "Write":
				// И старт, и возврат на доработку начинают новую итерацию до AI и его таймера.
				expression.ActivateType = bpm.ActivateTypeMethod
				expression.Method = bpm.Method(svc, prepare)
			case "HumanReview":
				reason := "iteration_limit"
				for _, source := range graph.Expressions {
					if source.ActivateType == bpm.ActivateTypeBoundary && slices.Contains(source.NextElements, id) {
						reason = "ai_timeout"
						break
					}
				}
				expression.ActivateType = bpm.ActivateTypeMethod
				expression.Method = bpm.Method(svc, func(_ context.Context, _ *services.Services, data *model.DataModel) error {
					data.Outcome = "needs_human"
					data.EscalationReason = reason
					return nil
				})
			}
		}
	}
	return processes
}

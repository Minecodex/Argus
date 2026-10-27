package argusdev

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/prometheus/prometheus/promql/parser"
)

// Aggregation can intentionally remove series identity labels. It is accepted
// only for an actual frozen aggregate expression and independently verified
// execution/resource/source scope; raw metrics keep the strict label oracle.
func plan8DefaultTargetAggregates(integration, panelID, targetID string) bool {
	for _, definition := range dashboard.BuiltinTemplates() {
		if definition.Key != integration+".overview" {
			continue
		}
		for _, panel := range definition.Spec.Panels {
			if panel.ID != panelID {
				continue
			}
			for _, target := range panel.Targets {
				if target.ID != targetID || target.SourceDefinition.DSL == nil {
					return false
				}
				expr, err := parser.NewParser(parser.Options{}).ParseExpr(target.SourceDefinition.DSL.Expression)
				if err != nil {
					return false
				}
				aggregate := false
				parser.Inspect(expr, func(n parser.Node, _ []parser.Node) error {
					if _, ok := n.(*parser.AggregateExpr); ok {
						aggregate = true
					}
					return nil
				})
				return aggregate
			}
		}
	}
	return false
}

func plan8DefaultViewScope(value map[string]any, row monitoring.Collection, sourceKeys []string) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var execution dashboard.Execution
	if json.Unmarshal(body, &execution) != nil || execution.ScopeSignature == "" || execution.ScopeEmpty || len(execution.Resources) != 1 || execution.Resources[0].Type != "monitoring_target" || execution.Resources[0].ID != row.Target.ID {
		return errors.New("default view execution escaped its explicit target scope")
	}
	allowed := map[string]bool{}
	for _, key := range sourceKeys {
		allowed[key] = true
	}
	for _, panel := range execution.Panels {
		if panel.Status != "success" {
			continue
		}
		if len(panel.Sources) == 0 {
			return errors.New("successful default panel omitted source provenance")
		}
		for _, source := range panel.Sources {
			if source.ResourceID != row.Target.ID || source.ResourceType != "monitoring_target" || !allowed[fmt.Sprintf("%s:%d", source.ID, source.Revision)] {
				return errors.New("default panel source escaped its target or collection")
			}
		}
	}
	return nil
}

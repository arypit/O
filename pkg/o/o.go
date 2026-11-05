// Package o is the public facade for embedding the O engine in other Go
// programs (the TS TUI drives the compiled binary, but this package is the
// in-process API).
package o

import (
	"github.com/adaop/o/internal/config"
	"github.com/adaop/o/internal/search"
	"github.com/adaop/o/internal/tools"
	"github.com/adaop/o/internal/tools/builtin"
)

// Version returns the configured O version.
func Version(dir string) string {
	cfg, err := config.Load(dir)
	if err != nil {
		return "0.0.0"
	}
	return cfg.Version
}

// NewRegistry builds the default tool registry: read-only tools, mutating
// tools and the search-based plan tool.
func NewRegistry() *tools.Registry {
	reg := tools.NewRegistry()
	builtin.RegisterDefaults(reg)
	builtin.RegisterMutating(reg)
	builtin.RegisterPlan(reg, PlanCandidates(reg))
	return reg
}

// PlanCandidates exposes the default planner candidate list.
func PlanCandidates(reg *tools.Registry) []search.PlanStep {
	return builtin.PlanCandidates(reg)
}

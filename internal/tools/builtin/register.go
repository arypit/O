package builtin

import (
	"github.com/adaop/o/internal/search"
	"github.com/adaop/o/internal/tools"
)

// RegisterDefaults adds the builtin read-only tools to the registry.
func RegisterDefaults(reg *tools.Registry) {
	reg.Add(ReadTool{})
	reg.Add(GlobTool{})
	reg.Add(GrepTool{})
	reg.Add(TodoTool{})
	reg.Add(WebFetchTool{})
}

// RegisterMutating adds the mutating tools (write, edit, bash).
func RegisterMutating(reg *tools.Registry) {
	reg.Add(WriteTool{})
	reg.Add(EditTool{})
	reg.Add(BashTool{})
}

// RegisterPlan adds the search-based planning tool with the given candidate actions.
func RegisterPlan(reg *tools.Registry, candidates []search.PlanStep) {
	reg.Add(PlanTool{Candidates: candidates})
}

// PlanCandidates builds the default candidate action list from a registry.
func PlanCandidates(reg *tools.Registry) []search.PlanStep {
	var out []search.PlanStep
	for _, t := range reg.All() {
		out = append(out, search.PlanStep{Tool: t.Name(), Summary: t.Name() + " action"})
	}
	return out
}

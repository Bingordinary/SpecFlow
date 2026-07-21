package impactsync

// ModuleBinding describes a module's governance state relevant to impact sync.
type ModuleBinding struct {
	Module        string
	ActiveLayer   string
	BindingIssues []string
}

// ScopedModule is a module within the impact sync scope.
type ScopedModule struct {
	Binding              ModuleBinding
	InvalidatingRuleRefs []string
	ExplicitFallbackScope bool
}

// Input is the input to the Apply function.
type Input struct {
	Modules []ScopedModule
}

// Result is the output of the Apply function.
type Result struct {
	ModuleResults []ModuleResult
}

// ModuleResult describes the result for one module.
type ModuleResult struct {
	Module             string
	ActiveLayer        string
	Outcome            string
	FallbackReasonCode string
	Diagnostics        []string
}

// Apply performs impact sync for the given input.
// In the simplified model, this reports which units are affected by rule changes
// without managing lifecycle state, process files, or _status.md.
func Apply(repoRoot string, input Input) (Result, error) {
	moduleResults := make([]ModuleResult, 0, len(input.Modules))
	for _, scoped := range input.Modules {
		result := reconcileModule(scoped)
		moduleResults = append(moduleResults, result)
	}

	return Result{
		ModuleResults: moduleResults,
	}, nil
}

func reconcileModule(scoped ScopedModule) ModuleResult {
	binding := scoped.Binding
	result := ModuleResult{
		Module:      binding.Module,
		ActiveLayer: binding.ActiveLayer,
		Outcome:     "unchanged",
		Diagnostics: append([]string{}, binding.BindingIssues...),
	}

	bindingIssue := len(binding.BindingIssues) > 0

	switch {
	case bindingIssue:
		result.Outcome = "affected"
		result.FallbackReasonCode = "binding_drift"
	case len(scoped.InvalidatingRuleRefs) > 0:
		result.Outcome = "affected"
		result.FallbackReasonCode = "rule_drift"
	case scoped.ExplicitFallbackScope:
		result.Outcome = "affected"
		result.FallbackReasonCode = "binding_drift"
	}

	return result
}


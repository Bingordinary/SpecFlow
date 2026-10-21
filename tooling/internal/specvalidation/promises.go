package specvalidation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"gopkg.in/yaml.v3"
)

// Promise is one normative contract declared by a unit.
//
// A promise owns no code and no verification activity. Acceptance items
// reference it and carry the evidence/code scope.
type Promise struct {
	ID        string
	Kind      string
	Summary   string
	Contract  map[string]any
	DependsOn []string
	// OpaqueReason is required when Kind is "opaque": the author's stated
	// reason no structured kind applies. It is justification metadata, not
	// contract, so it stays out of the fingerprint.
	OpaqueReason string
	// Raw carries every top-level field so schema checks can reject
	// verification or code fields without a second parser.
	Raw map[string]any
}

var (
	promiseIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	promiseKinds     = map[string]bool{
		"request_response": true,
		"event":            true,
		"data_schema":      true,
		"state_machine":    true,
		"opaque":           true,
	}
	promiseForbiddenTopLevel = map[string]bool{
		"implementation_surface": true,
		"affects":                true,
		"verification_method":    true,
		"pass_condition":         true,
		"verified_by":            true,
		"consumers":              true,
		"consumer":               true,
		"code":                   true,
		"files":                  true,
		"tests":                  true,
		"evidence":               true,
	}
	// promiseAllowedTopLevel is the closed set of fields the deployed promise
	// format defines. An unknown field is not part of the fingerprint and could
	// otherwise carry a second, unfingerprinted contract.
	promiseAllowedTopLevel = map[string]bool{
		"id":            true,
		"kind":          true,
		"summary":       true,
		"contract":      true,
		"opaque_reason": true,
		"depends_on":    true,
	}
	// promiseContractFields is the closed contract shape per kind. An unknown
	// contract key is rejected: the fingerprint covers `contract`, so an
	// unvalidated key would be contract truth outside the model's shape.
	promiseContractFields = map[string]map[string]bool{
		"request_response": {
			"request": true, "response": true, "errors": true, "timing": true,
		},
		"event": {
			"name": true, "payload": true, "delivery": true, "ordering": true,
		},
		"data_schema": {
			"fields": true,
		},
		"state_machine": {
			"states": true, "initial": true, "transitions": true,
		},
		"opaque": {
			"text": true,
		},
	}
)

// HasPromisesSection reports whether the spec declares the promise layer.
func HasPromisesSection(content string) bool {
	_, ok := contenthash.LocateSectionRegion(specpaths.NormalizeText(content), "Promises")
	return ok
}

// CheckUnitSpecShape enforces the fixed three-section unit spec shape.
//
// The promise model needs stable section locations for mechanical parsing and
// region pinning: Design Narrative, Promises, Acceptance Items, in that order.
func CheckUnitSpecShape(content string) CheckResult {
	content = specpaths.NormalizeText(content)
	result := CheckResult{Name: "Unit spec shape", Status: Fail}

	regions := contenthash.SectionRegions(content)
	if len(regions) == 0 {
		result.Details = "spec has no sections"
		return result
	}
	frontmatter := ""
	var headings []string
	byHeading := map[string]string{}
	for _, region := range regions {
		if region.Heading == "" {
			frontmatter = region.Text
			continue
		}
		headings = append(headings, region.Heading)
		byHeading[region.Heading] = region.Text
	}

	if !strings.HasPrefix(strings.TrimSpace(frontmatter), "---") {
		result.Details = "unit spec must start with frontmatter"
		return result
	}

	want := []string{"Design Narrative", "Promises", "Acceptance Items"}
	if len(headings) != len(want) {
		result.Details = fmt.Sprintf("expected exactly 3 ## sections (%s), found %d: %s",
			strings.Join(want, ", "), len(headings), strings.Join(headings, ", "))
		return result
	}
	for i, heading := range want {
		if headings[i] != heading {
			result.Details = fmt.Sprintf("section %d must be %q, found %q", i+1, heading, headings[i])
			return result
		}
	}

	for _, heading := range []string{"Promises", "Acceptance Items"} {
		if hasDeeperHeading(byHeading[heading]) {
			result.Details = fmt.Sprintf("%q must contain structured content only; use no ### subheadings", heading)
			return result
		}
	}

	promisesSection := byHeading["Promises"]
	if !hasTopLevelMarker(promisesSection, "promises") {
		result.Details = `"Promises" section must contain a top-level promises: list`
		return result
	}
	if _, ok := contenthash.AcceptanceItemsRegion(byHeading["Acceptance Items"]); !ok {
		result.Details = `"Acceptance Items" section must contain acceptance_item_set:`
		return result
	}

	result.Status = Pass
	result.Details = "Design Narrative, Promises, Acceptance Items are present once and in order"
	return result
}

// hasDeeperHeading reports whether a section text contains a `###` heading.
func hasDeeperHeading(section string) bool {
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "### ") {
			return true
		}
	}
	return false
}

// hasTopLevelMarker reports whether a section contains `key:` at column zero,
// outside a fenced code block.
func hasTopLevelMarker(section, key string) bool {
	inFence := false
	var fenceChar byte
	fenceLen := 0
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		if inFence {
			if strings.HasPrefix(trimmed, strings.Repeat(string(fenceChar), fenceLen)) {
				inFence = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = true
			fenceChar = trimmed[0]
			fenceLen = 0
			for fenceLen < len(trimmed) && trimmed[fenceLen] == fenceChar {
				fenceLen++
			}
			continue
		}
		if strings.HasPrefix(line, key+":") {
			return true
		}
	}
	return false
}

// ParsePromises parses the `## Promises` section. It returns every promise it
// can read plus a list of structural issues. Schema and coverage checks run
// separately so one malformed entry does not hide the others.
func ParsePromises(content string) ([]Promise, []string) {
	content = specpaths.NormalizeText(content)
	section, ok := contenthash.LocateSectionRegion(content, "Promises")
	if !ok {
		return nil, []string{"## Promises section not found"}
	}

	body := stripSectionHeading(section.Text)
	var root map[string]any
	if err := yaml.Unmarshal([]byte(body), &root); err != nil {
		return nil, []string{fmt.Sprintf("Promises section is not valid YAML: %v", err)}
	}
	rawList, ok := root["promises"]
	if !ok {
		return nil, []string{"Promises section must contain a top-level promises: list"}
	}
	list, ok := rawList.([]any)
	if !ok {
		return nil, []string{"promises: must be a YAML list"}
	}

	var promises []Promise
	var issues []string
	for i, raw := range list {
		entry, ok := raw.(map[string]any)
		if !ok {
			issues = append(issues, fmt.Sprintf("promise %d: entry must be a mapping", i+1))
			continue
		}
		normalized := normalizeYAMLMap(entry)
		p := Promise{Raw: normalized}
		p.ID, _ = normalized["id"].(string)
		p.Kind, _ = normalized["kind"].(string)
		p.Summary, _ = normalized["summary"].(string)
		p.OpaqueReason, _ = normalized["opaque_reason"].(string)
		if contract, ok := normalized["contract"].(map[string]any); ok {
			p.Contract = contract
		}
		p.DependsOn, issues = parsePromiseStringList(normalized["depends_on"], fmt.Sprintf("promise %q depends_on", p.ID), issues)
		promises = append(promises, p)
	}
	return promises, issues
}

// stripSectionHeading removes the first line of a section region.
func stripSectionHeading(section string) string {
	if i := strings.IndexByte(section, '\n'); i >= 0 {
		return section[i+1:]
	}
	return ""
}

// parsePromiseStringList accepts a scalar or a YAML list.
func parsePromiseStringList(raw any, label string, issues []string) ([]string, []string) {
	if raw == nil {
		return nil, issues
	}
	switch value := raw.(type) {
	case string:
		if strings.TrimSpace(value) == "" || value == "none" {
			return nil, issues
		}
		return []string{strings.TrimSpace(value)}, issues
	case []any:
		var out []string
		for _, entry := range value {
			text, ok := entry.(string)
			if !ok || strings.TrimSpace(text) == "" {
				issues = append(issues, fmt.Sprintf("%s: entries must be non-empty strings", label))
				continue
			}
			out = append(out, strings.TrimSpace(text))
		}
		return out, issues
	default:
		issues = append(issues, fmt.Sprintf("%s: must be a string or a list of strings", label))
		return nil, issues
	}
}

// CheckPromiseSchema validates promise fields and the no-code/no-verification
// boundary. Cross-promise resolution is owned by the dependency graph.
func CheckPromiseSchema(content, unitName string) CheckResult {
	result := CheckResult{Name: "Promise schema", Status: Fail}
	promises, issues := ParsePromises(content)
	if len(issues) > 0 {
		result.Details = strings.Join(issues, "; ")
		return result
	}
	if len(promises) == 0 {
		result.Status = Pass
		result.Details = "no promises declared"
		return result
	}

	seen := map[string]bool{}
	for i, p := range promises {
		label := fmt.Sprintf("promise %d", i+1)
		if p.ID == "" {
			issues = append(issues, label+": missing or empty id")
			continue
		}
		label = fmt.Sprintf("promise %q", p.ID)
		if !promiseIDPattern.MatchString(p.ID) {
			issues = append(issues, label+": id must use letters, digits, '_', '-', or '.'")
		}
		if !strings.HasPrefix(p.ID, unitName+".") {
			issues = append(issues, fmt.Sprintf("%s: unit-owned promise id must start with %q", label, unitName+"."))
		}
		if seen[p.ID] {
			issues = append(issues, label+": duplicate id")
		}
		seen[p.ID] = true

		if !promiseKinds[p.Kind] {
			issues = append(issues, fmt.Sprintf("%s: invalid kind %q; must be request_response, event, data_schema, state_machine, or opaque", label, p.Kind))
		}
		if strings.TrimSpace(p.Summary) == "" {
			issues = append(issues, label+": missing or empty summary")
		} else if strings.Contains(p.Summary, "\n") {
			issues = append(issues, label+": summary must be one line")
		}
		if len(p.Contract) == 0 && p.Kind != "opaque" {
			issues = append(issues, label+": missing or empty contract")
		}
		if _, present := p.Raw["opaque_reason"]; present && p.Kind != "opaque" {
			issues = append(issues, label+": opaque_reason is only allowed on kind: opaque")
		}
		if p.Kind == "opaque" && strings.TrimSpace(p.OpaqueReason) == "" {
			issues = append(issues, label+": opaque promise requires a non-empty opaque_reason stating why no structured kind applies")
		}
		if len(p.Contract) > 0 || p.Kind == "opaque" {
			issues = append(issues, validatePromiseContract(label, p.Kind, p.Contract)...)
		}
		for field := range p.Raw {
			switch {
			case promiseForbiddenTopLevel[field]:
				issues = append(issues, fmt.Sprintf("%s: top-level field %q is not allowed on a promise", label, field))
			case !promiseAllowedTopLevel[field]:
				issues = append(issues, fmt.Sprintf("%s: unknown top-level field %q; allowed fields are id, kind, summary, contract, opaque_reason, depends_on", label, field))
			}
		}
		depSeen := map[string]bool{}
		for _, dep := range p.DependsOn {
			if dep == p.ID {
				issues = append(issues, label+": depends_on must not reference itself")
			} else if provider := PromiseProvider(dep); provider == unitName {
				issues = append(issues, fmt.Sprintf("%s: depends_on %q is owned by this unit; a promise depends on another unit's contract, not on a promise in the same unit", label, dep))
			}
			if depSeen[dep] {
				issues = append(issues, fmt.Sprintf("%s: duplicate depends_on entry %q", label, dep))
			}
			depSeen[dep] = true
		}
	}

	if len(issues) > 0 {
		result.Details = strings.Join(issues, "; ")
		return result
	}
	result.Status = Pass
	result.Details = fmt.Sprintf("%d promise(s) have valid schema", len(promises))
	return result
}

// CheckPromiseCoverage enforces the promise/acceptance-item relation:
// every contract item references a promise, every promise is covered by at
// least one item, and non-contract items are explicit.
func CheckPromiseCoverage(content, unitName string) CheckResult {
	result := CheckResult{Name: "Promise coverage", Status: Fail}
	promises, issues := ParsePromises(content)
	if len(issues) > 0 {
		result.Details = strings.Join(issues, "; ")
		return result
	}
	promiseIDs := map[string]bool{}
	for _, p := range promises {
		promiseIDs[p.ID] = true
	}

	items := parseAcceptanceItems(content)
	if len(items) == 0 {
		if len(promises) == 0 {
			result.Status = Pass
			result.Details = "no promises or acceptance items declared"
			return result
		}
		result.Details = "acceptance_item_set exists but no items found with - id:"
		return result
	}

	covered := map[string]bool{}
	for _, item := range items {
		label := fmt.Sprintf("acceptance item %q", item.id)
		promise := strings.TrimSpace(item.fields["promise"])
		nonPromise := strings.EqualFold(strings.TrimSpace(item.fields["non_promise"]), "true")
		reason := strings.TrimSpace(item.fields["non_promise_reason"])

		switch {
		case promise != "" && nonPromise:
			issues = append(issues, label+": cannot declare both promise and non_promise")
		case promise != "":
			if !promiseIDs[promise] {
				issues = append(issues, fmt.Sprintf("%s: references unknown promise %q", label, promise))
				continue
			}
			if !strings.HasPrefix(promise, unitName+".") {
				issues = append(issues, fmt.Sprintf("%s: verifies foreign promise %q; consumer dependencies belong on the consumer promise's depends_on", label, promise))
				continue
			}
			covered[promise] = true
		case nonPromise:
			if reason == "" {
				issues = append(issues, label+": non_promise item requires non_promise_reason")
			}
		default:
			issues = append(issues, label+": must declare promise or non_promise: true")
		}
	}

	for _, p := range promises {
		if !covered[p.ID] {
			issues = append(issues, fmt.Sprintf("promise %q: not covered by any acceptance item", p.ID))
		}
	}

	if len(issues) > 0 {
		result.Details = strings.Join(issues, "; ")
		return result
	}
	result.Status = Pass
	result.Details = fmt.Sprintf("%d promise(s) covered by %d acceptance item(s)", len(promises), len(items))
	return result
}

// CheckPromiseSpec runs the promise-model checks. Provider units for external
// promise dependencies are derived from depends_on by the gate planner.
func CheckPromiseSpec(content, unitName string) []CheckResult {
	return []CheckResult{
		CheckUnitSpecShape(content),
		CheckPromiseSchema(content, unitName),
		CheckPromiseCoverage(content, unitName),
	}
}

// validatePromiseContract checks the required shape of one promise kind. It
// validates structure only; semantic compatibility remains a later diff. The
// contract keys are closed per kind: an unknown key is not part of the
// documented shape and would otherwise enter the fingerprint as contract truth
// the model never defined.
func validatePromiseContract(label, kind string, contract map[string]any) []string {
	var shapeIssues []string
	if allowed, ok := promiseContractFields[kind]; ok {
		for field := range contract {
			if !allowed[field] {
				shapeIssues = append(shapeIssues, fmt.Sprintf("%s: unknown %s contract field %q", label, kind, field))
			}
		}
	}
	switch kind {
	case "opaque":
		issues := append([]string{}, shapeIssues...)
		text, ok := contract["text"].(string)
		if !ok || strings.TrimSpace(text) == "" {
			issues = append(issues, label+": opaque promise requires contract.text")
		}
		if len(contract) != 1 {
			issues = append(issues, label+": opaque promise contract may contain only text")
		}
		return issues
	case "request_response":
		issues := append([]string{}, shapeIssues...)
		if response, ok := contract["response"]; !ok {
			issues = append(issues, label+": request_response requires contract.response")
		} else if fields, ok := response.(map[string]any); !ok || len(fields) == 0 {
			issues = append(issues, label+": request_response contract.response must be a non-empty field map")
		} else {
			issues = append(issues, validatePromiseFields(label+".response", fields)...)
		}
		if request, ok := contract["request"]; ok {
			fields, ok := request.(map[string]any)
			if !ok {
				issues = append(issues, label+": request_response contract.request must be a field map")
			} else {
				issues = append(issues, validatePromiseFields(label+".request", fields)...)
			}
		}
		if errorsValue, ok := contract["errors"]; ok {
			errors, ok := errorsValue.(map[string]any)
			if !ok {
				issues = append(issues, label+": request_response contract.errors must be a map")
			} else {
				for code, meaning := range errors {
					if strings.TrimSpace(fmt.Sprint(code)) == "" {
						issues = append(issues, label+": request_response contract.errors has an empty code")
					}
					if text, ok := meaning.(string); !ok || strings.TrimSpace(text) == "" {
						issues = append(issues, fmt.Sprintf("%s: request_response error %v must be a non-empty string", label, code))
					}
				}
			}
		}
		if timingValue, ok := contract["timing"]; ok {
			timing, ok := timingValue.(map[string]any)
			if !ok {
				issues = append(issues, label+": request_response contract.timing must be a map")
			} else if mode, ok := timing["mode"]; ok {
				if text, ok := mode.(string); !ok || strings.TrimSpace(text) == "" {
					issues = append(issues, label+": request_response contract.timing.mode must be a non-empty string")
				}
			}
		}
		return issues
	case "event":
		issues := append([]string{}, shapeIssues...)
		if name, ok := contract["name"].(string); !ok || strings.TrimSpace(name) == "" {
			issues = append(issues, label+": event requires contract.name")
		}
		if payload, ok := contract["payload"]; !ok {
			issues = append(issues, label+": event requires contract.payload")
		} else if fields, ok := payload.(map[string]any); !ok {
			issues = append(issues, label+": event contract.payload must be a field map")
		} else {
			issues = append(issues, validatePromiseFields(label+".payload", fields)...)
		}
		return issues
	case "data_schema":
		issues := append([]string{}, shapeIssues...)
		fields, ok := contract["fields"].(map[string]any)
		if !ok || len(fields) == 0 {
			issues = append(issues, label+": data_schema requires a non-empty contract.fields map")
			return issues
		}
		return append(issues, validatePromiseFields(label+".fields", fields)...)
	case "state_machine":
		issues := append([]string{}, shapeIssues...)
		states, ok := stringList(contract["states"])
		if !ok || len(states) == 0 {
			issues = append(issues, label+": state_machine requires a non-empty contract.states list")
		} else if duplicates := duplicateStrings(states); len(duplicates) > 0 {
			issues = append(issues, fmt.Sprintf("%s: state_machine states duplicate %s", label, strings.Join(duplicates, ", ")))
		}
		stateSet := map[string]bool{}
		for _, state := range states {
			stateSet[state] = true
		}
		if initial, ok := contract["initial"].(string); !ok || strings.TrimSpace(initial) == "" {
			issues = append(issues, label+": state_machine requires contract.initial")
		} else if len(stateSet) > 0 && !stateSet[initial] {
			issues = append(issues, label+": state_machine initial state is not in states")
		}
		transitions, ok := contract["transitions"].([]any)
		if !ok || len(transitions) == 0 {
			issues = append(issues, label+": state_machine requires a non-empty contract.transitions list")
			return issues
		}
		for i, raw := range transitions {
			entry, ok := raw.(map[string]any)
			if !ok {
				issues = append(issues, fmt.Sprintf("%s: state_machine transition %d must be a map", label, i+1))
				continue
			}
			for _, field := range []string{"from", "event", "to"} {
				value, ok := entry[field].(string)
				if !ok || strings.TrimSpace(value) == "" {
					issues = append(issues, fmt.Sprintf("%s: state_machine transition %d requires %s", label, i+1, field))
				}
			}
			if from, ok := entry["from"].(string); ok && len(stateSet) > 0 && !stateSet[from] {
				issues = append(issues, fmt.Sprintf("%s: state_machine transition %d from state %q is not in states", label, i+1, from))
			}
			if to, ok := entry["to"].(string); ok && len(stateSet) > 0 && !stateSet[to] {
				issues = append(issues, fmt.Sprintf("%s: state_machine transition %d to state %q is not in states", label, i+1, to))
			}
		}
		return issues
	default:
		return nil
	}
}

func validatePromiseFields(label string, fields map[string]any) []string {
	var issues []string
	for name, raw := range fields {
		if strings.TrimSpace(name) == "" {
			issues = append(issues, label+": empty field name")
			continue
		}
		switch value := raw.(type) {
		case string:
			if strings.TrimSpace(value) == "" {
				issues = append(issues, fmt.Sprintf("%s.%s: type must be non-empty", label, name))
			}
		case map[string]any:
			typeName, ok := value["type"].(string)
			if !ok || strings.TrimSpace(typeName) == "" {
				issues = append(issues, fmt.Sprintf("%s.%s: missing or empty type", label, name))
			}
			for _, flag := range []string{"required", "nullable"} {
				if rawFlag, ok := value[flag]; ok {
					if _, ok := rawFlag.(bool); !ok {
						issues = append(issues, fmt.Sprintf("%s.%s: %s must be a boolean", label, name, flag))
					}
				}
			}
		default:
			issues = append(issues, fmt.Sprintf("%s.%s: field spec must be a type string or a map with type", label, name))
		}
	}
	return issues
}

func stringList(raw any) ([]string, bool) {
	list, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(list))
	for _, entry := range list {
		text, ok := entry.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, false
		}
		out = append(out, text)
	}
	return out, true
}

func duplicateStrings(values []string) []string {
	seen := map[string]bool{}
	var duplicates []string
	for _, value := range values {
		if seen[value] {
			duplicates = append(duplicates, value)
			continue
		}
		seen[value] = true
	}
	return duplicates
}

// PromiseFingerprint returns the canonical fingerprint of a promise contract.
// The fingerprint covers kind + contract only. Identity, summary, and graph
// edges are excluded so a provider can rename a promise or update prose
// without reopening consumers.
func PromiseFingerprint(p Promise) (string, error) {
	payload := map[string]any{
		"kind":     p.Kind,
		"contract": normalizeYAMLValue(p.Contract),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode promise %q: %w", p.ID, err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// normalizeYAMLMap recursively converts YAML maps with non-string keys into
// map[string]any so JSON encoding is deterministic.
func normalizeYAMLMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = normalizeYAMLValue(value)
	}
	return out
}

func normalizeYAMLValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return normalizeYAMLMap(v)
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, entry := range v {
			out[fmt.Sprint(key)] = normalizeYAMLValue(entry)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, entry := range v {
			out[i] = normalizeYAMLValue(entry)
		}
		return out
	default:
		return value
	}
}

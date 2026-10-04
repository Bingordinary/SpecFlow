package specvalidation

import (
	"strings"
	"testing"
)

func schemaItem(id string) string {
	return "  - id: " + id + "\n" +
		"    description: Given a request, when inspected, then it is accepted.\n" +
		"    verification_type: testable\n" +
		"    verification_surface: api\n" +
		"    implementation_surface: <pending>\n" +
		"    verification_method: Run the request test.\n" +
		"    pass_condition: The request is accepted.\n" +
		"    runnable: yes\n"
}

func TestAcceptanceSchemaChecksEveryItem(t *testing.T) {
	fields := []string{"description", "verification_type", "verification_surface", "implementation_surface", "verification_method", "pass_condition", "runnable"}
	for _, field := range fields {
		for _, position := range []string{"first", "second"} {
			t.Run(position+"/"+field, func(t *testing.T) {
				items := []string{schemaItem("first"), schemaItem("second")}
				index := 0
				if position == "second" {
					index = 1
				}
				lines := strings.Split(items[index], "\n")
				var kept []string
				for _, line := range lines {
					if !strings.HasPrefix(line, "    "+field+":") {
						kept = append(kept, line)
					}
				}
				items[index] = strings.Join(kept, "\n")
				root := newRepo(t)
				writeCandidate(t, root, "demo", "acceptance_item_set:\n"+strings.Join(items, ""))
				result := checkAcceptanceItems(root, "demo")
				if result.Status != Fail || !strings.Contains(result.Details, position) || !strings.Contains(result.Details, field) {
					t.Fatalf("missing own field was not identified: %+v", result)
				}
			})
		}
	}
}

func TestAcceptanceSchemaFieldBoundariesAndValues(t *testing.T) {
	complete := "acceptance_item_set:\n" + schemaItem("first")
	method := "    verification_method: Run the request test.\n"
	missing := strings.Replace(complete, method, "", 1)
	for _, tc := range []struct {
		name, content, field string
		pass                 bool
	}{
		{"complete", complete, "", true},
		{"inspectable", strings.Replace(complete, "testable", "inspectable", 1), "", true},
		{"reviewable", strings.Replace(complete, "testable", "reviewable", 1), "", true},
		{"nested whole list", strings.ReplaceAll(complete, "\n", "\n  "), "", true},
		{"CRLF", strings.ReplaceAll(complete, "\n", "\r\n"), "", true},
		{"quoted enum and comment", strings.Replace(complete, "testable", "\"testable\" # chosen method", 1), "", true},
		{"quoted block character", strings.Replace(complete, "Run the request test.", "\"|\"", 1), "", true},
		{"block description", strings.Replace(complete, "description: Given a request, when inspected, then it is accepted.", "description: |\n      Given a request\n      When inspected\n      Then it is accepted", 1), "", true},
		{"not runnable with reason", strings.Replace(complete, "runnable: yes", "runnable: no\n    not_runnable_reason: Requires a reviewer.", 1), "", true},
		{"later section", missing + "\n## Notes\n" + method, "verification_method", false},
		{"nested field", missing + "    affects:\n      verification_method: Nested text\n", "verification_method", false},
		{"fenced field", missing + "    ```yaml\n" + method + "    ```\n", "verification_method", false},
		{"field in description", strings.Replace(missing, "description: Given a request, when inspected, then it is accepted.", "description: |\n      verification_method: example", 1), "verification_method", false},
		{"empty field", strings.Replace(complete, method, "    verification_method: # no method\n", 1), "verification_method", false},
		{"quoted empty field", strings.Replace(complete, method, "    verification_method: \"\"\n", 1), "verification_method", false},
		{"empty block", strings.Replace(complete, method, "    verification_method: |\n", 1), "verification_method", false},
		{"invalid type", strings.Replace(complete, "testable", "auto", 1), "verification_type", false},
		{"invalid runnable", strings.Replace(complete, "runnable: yes", "runnable: true", 1), "runnable", false},
		{"missing reason", strings.Replace(complete, "runnable: yes", "runnable: no", 1), "not_runnable_reason", false},
		{"duplicate field", complete + method, "verification_method", false},
		{"duplicate id", complete + schemaItem("first"), "id", false},
		{"empty id", strings.Replace(complete, "- id: first", "- id:", 1), "id", false},
		{"only fenced set", "```yaml\n" + complete + "```\n", "acceptance_item_set", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newRepo(t)
			writeCandidate(t, root, "demo", tc.content)
			result := checkAcceptanceItems(root, "demo")
			if (result.Status == Pass) != tc.pass || (!tc.pass && !strings.Contains(result.Details, tc.field)) {
				t.Fatalf("unexpected schema result: %+v", result)
			}
		})
	}
}

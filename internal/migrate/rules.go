package migrate

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/liemle3893/go-tryve/internal/assertion"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

// Rule identifiers, used to group a report.
const (
	RuleDroppedField     = "assertion on a result field was never evaluated"
	RuleDroppedRowColumn = "row/column assertion was never evaluated"
	RuleDroppedOperator  = "operator was not recognised and was dropped"
	RuleExitCodeSuppress = "exitCode assertion also suppressed the exit-code check"
	RuleArrayBodyRoot    = "\"$\" addressed the result, not an array body"
	RuleTypedValue       = "a lone {{expression}} rendered to text"
	RuleObjectRender     = "an object rendered with Go's %v, not as JSON"
	RuleQuotedArg        = "a quoted builtin argument kept its quotes"
	RuleStepTimeout      = "step timeout was ignored"
	RuleStepSkip         = "step skip was ignored"
	RuleCountAggregate   = "count reported rows returned, not the COUNT(*) value"
	RuleShellUnbounded   = "shell command ran unbounded"
	RuleFindOneShape     = "findOne returned only {document: …}"
)

// httpAssertKeys are the assertion keys that were always handled.
var httpAssertKeys = map[string]bool{
	"status": true, "statusRange": true, "headers": true,
	"json": true, "body": true, "duration": true,
}

// legacyOperators is the operator set the previous release evaluated. Anything
// else in an assertion position was silently discarded.
var legacyOperators = map[string]bool{
	"equals": true, "notEquals": true, "contains": true, "notContains": true,
	"matches": true, "type": true, "exists": true, "notExists": true,
	"isNull": true, "isNotNull": true, "greaterThan": true, "lessThan": true,
	"greaterThanOrEqual": true, "lessThanOrEqual": true, "length": true,
	"isEmpty": true, "notEmpty": true, "hasProperty": true, "notHasProperty": true,
}

// soleExpr matches a value consisting of exactly one {{…}} expression.
var soleExpr = regexp.MustCompile(`^\s*\{\{[^{}]*\}\}\s*$`)

// quotedArg matches a builtin call with a quoted argument.
var quotedArg = regexp.MustCompile(`\{\{\s*\$\w+\([^)]*["'][^)]*\)\s*\}\}`)

// countAggregate matches SQL that aggregates rather than selecting rows.
var countAggregate = regexp.MustCompile(`(?i)\bcount\s*\(`)

// assertionFindings reports assertion forms whose evaluation changes.
func assertionFindings(path string, step *yaml.Node) []Finding {
	assertNode := mappingValue(step, "assert")
	if assertNode == nil {
		return nil
	}

	var findings []Finding
	area := tryve.CompatAssertions

	switch assertNode.Kind {
	case yaml.MappingNode:
		hasExitCode := false
		for i := 0; i+1 < len(assertNode.Content); i += 2 {
			key, val := assertNode.Content[i].Value, assertNode.Content[i+1]

			if httpAssertKeys[key] {
				if key == "json" {
					findings = append(findings, jsonBlockFindings(path, val)...)
				}
				continue
			}
			if key == "path" {
				continue
			}
			if legacyOperators[key] {
				continue
			}

			// Anything else was a field assertion that did not exist yet.
			if key == "exitCode" {
				hasExitCode = true
				findings = append(findings, finding(path, assertNode.Content[i], area, WillChange,
					RuleExitCodeSuppress,
					fmtDetail("`exitCode: %s` was discarded, and its presence disabled the automatic non-zero-exit failure",
						describeNode(val))))
				continue
			}
			findings = append(findings, finding(path, assertNode.Content[i], area, WillChange,
				RuleDroppedField, fmtDetail("`%s:` was discarded", key)))
		}
		_ = hasExitCode

	case yaml.SequenceNode:
		for _, item := range assertNode.Content {
			findings = append(findings, sliceItemFindings(path, item)...)
		}
	}

	return findings
}

// jsonBlockFindings inspects the items of an HTTP `json:` assertion block.
func jsonBlockFindings(path string, seq *yaml.Node) []Finding {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil
	}
	var findings []Finding
	for _, item := range seq.Content {
		findings = append(findings, sliceItemFindings(path, item)...)

		// When the response body is an array, "$" changes from addressing the
		// whole result to addressing the array. Two shapes are affected:
		// a path rooted directly at the array, and a path naming a field of the
		// result envelope — which could only ever have resolved because the body
		// was not an object.
		if p := mappingValue(item, "path"); p != nil {
			v := strings.TrimSpace(p.Value)
			switch {
			case v == "$" || strings.HasPrefix(v, "$["):
				findings = append(findings, finding(path, p, tryve.CompatAssertions, MayChange,
					RuleArrayBodyRoot,
					fmtDetail("`path: %q` addresses the array body instead of the whole result, when the body is an array", v)))
			case envelopeField(v):
				findings = append(findings, finding(path, p, tryve.CompatAssertions, MayChange,
					RuleArrayBodyRoot,
					fmtDetail("`path: %q` names a result field, which only resolves inside `json:` when the body is not an object", v)))
			}
		}
	}
	return findings
}

// sliceItemFindings inspects one item of a list-form assertion.
func sliceItemFindings(path string, item *yaml.Node) []Finding {
	if item == nil || item.Kind != yaml.MappingNode {
		return nil
	}
	area := tryve.CompatAssertions
	keys := mappingKeys(item)

	hasPath := false
	hasColumn := false
	for _, k := range keys {
		switch k {
		case "path":
			hasPath = true
		case "column":
			hasColumn = true
		}
	}

	var findings []Finding

	if hasColumn {
		return append(findings, finding(path, item, area, WillChange, RuleDroppedRowColumn,
			fmtDetail("`{row/column}` item %s was discarded entirely", describeNode(item))))
	}

	if !hasPath {
		// Without "path" the whole item matched no handler.
		return append(findings, finding(path, item, area, WillChange, RuleDroppedField,
			fmtDetail("item %s had no `path` and was discarded", describeNode(item))))
	}

	for i := 0; i+1 < len(item.Content); i += 2 {
		key := item.Content[i].Value
		if key == "path" || key == "row" || key == "column" {
			continue
		}
		if legacyOperators[key] {
			continue
		}
		if _, known := assertion.CanonicalOperator(key); known {
			findings = append(findings, finding(path, item.Content[i], area, WillChange,
				RuleDroppedOperator, fmtDetail("operator `%s` was discarded; it is evaluated now", key)))
			continue
		}
		findings = append(findings, finding(path, item.Content[i], area, WillChange,
			RuleDroppedOperator,
			fmtDetail("`%s` is not an operator; it was discarded and now fails the assertion", key)))
	}

	return findings
}

// interpolationFindings reports expression sites whose resolved form changes.
func interpolationFindings(path string, step *yaml.Node) []Finding {
	area := tryve.CompatInterpolation
	var findings []Finding

	// Only positions that are serialised with their type can change meaning: a
	// request body, bound SQL parameters, and an assertion's expected value. A
	// URL or a shell command is text either way.
	for _, key := range []string{"body", "params", "assert", "message", "messages", "document", "documents", "value"} {
		node := mappingValue(step, key)
		if node == nil {
			continue
		}
		var scalars []*yaml.Node
		scalarStrings(node, &scalars)
		for _, sc := range scalars {
			if soleExpr.MatchString(sc.Value) {
				findings = append(findings, finding(path, sc, area, MayChange, RuleTypedValue,
					fmtDetail("`%s` keeps its resolved type instead of becoming text — only differs if it resolves to a number, boolean, object, or null",
						strings.TrimSpace(sc.Value))))
			}
		}
	}

	// Quoted builtin arguments lose their quotes.
	var allScalars []*yaml.Node
	scalarStrings(step, &allScalars)
	for _, sc := range allScalars {
		if quotedArg.MatchString(sc.Value) {
			findings = append(findings, finding(path, sc, area, WillChange, RuleQuotedArg,
				fmtDetail("quoted builtin argument in %q now resolves without its quotes", truncate(sc.Value, 60))))
		}
	}

	return findings
}

// executionFindings reports step fields that begin taking effect.
func executionFindings(path string, step *yaml.Node) []Finding {
	area := tryve.CompatExecution
	var findings []Finding

	if t := mappingValue(step, "timeout"); t != nil {
		// kafka and eventhub always read `timeout` as their own deadline, so it
		// was never ignored for those adapters.
		switch adapterOf(step) {
		case "kafka", "eventhub":
		default:
			findings = append(findings, finding(path, t, area, MayChange, RuleStepTimeout,
				fmtDetail("`timeout: %s` is enforced now; the step fails if it takes longer", t.Value)))
		}
	}

	if sk := mappingValue(step, "skip"); sk != nil && sk.Value == "true" {
		findings = append(findings, finding(path, sk, area, WillChange, RuleStepSkip,
			"`skip: true` is honoured now; this step stops running"))
	}

	return findings
}

// adapterFindings reports adapter result shapes and defaults that change.
func adapterFindings(path string, step *yaml.Node) []Finding {
	area := tryve.CompatAdapters
	var findings []Finding

	switch adapterOf(step) {
	case "postgresql", "postgres":
		if actionOf(step) == "count" {
			if sql := mappingValue(step, "sql"); sql != nil && countAggregate.MatchString(sql.Value) {
				findings = append(findings, finding(path, sql, area, WillChange, RuleCountAggregate,
					"`count` returns the COUNT(*) value now, not the number of rows the query returned (which was always 1)"))
			}
		}

	case "mongodb":
		if actionOf(step) == "findOne" {
			// Only a path into "document" is unaffected; anything else was nil.
			findings = append(findings, finding(path, step, area, MayChange, RuleFindOneShape,
				"`findOne` exposes the document's fields at the top level now; `$.document.…` paths keep working"))
		}

	case "shell":
		if mappingValue(step, "timeout") == nil {
			findings = append(findings, finding(path, step, area, MayChange, RuleShellUnbounded,
				"this command gets a 60s default timeout now; it was unbounded"))
		}
	}

	return findings
}

// envelopeField reports whether a JSONPath names a top-level field of the step
// result rather than of the response body.
func envelopeField(path string) bool {
	trimmed := strings.TrimPrefix(strings.TrimSpace(path), "$")
	trimmed = strings.TrimPrefix(trimmed, ".")
	head := trimmed
	for _, sep := range []string{".", "["} {
		if i := strings.Index(head, sep); i >= 0 {
			head = head[:i]
		}
	}
	switch head {
	case "status", "statusText", "headers", "body", "duration":
		return true
	}
	return false
}

// truncate shortens a string for a report line.
func truncate(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}

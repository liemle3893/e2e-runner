// Package migrate analyses a suite for behaviour that differs between
// compatibility levels, and records the per-file pins that let a large suite
// move one file at a time.
package migrate

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/liemle3893/go-tryve/internal/tryve"
)

// Certainty distinguishes a difference that is decidable from the file alone
// from one that depends on values only seen at run time.
type Certainty string

const (
	// WillChange means the file provably behaves differently at the new level.
	WillChange Certainty = "will change"
	// MayChange means the difference depends on runtime values — the type of a
	// captured value, the response body's shape, how long a command takes.
	MayChange Certainty = "may change"
)

// Finding is one construct in one file whose behaviour differs between levels.
type Finding struct {
	File      string
	Area      tryve.CompatArea
	Certainty Certainty
	// Rule identifies the kind of difference, for grouping in a report.
	Rule string
	// Detail describes this occurrence.
	Detail string
	// Line is the 1-indexed line the construct starts on, or 0 when unknown.
	Line int
}

// Report aggregates the findings for a suite.
type Report struct {
	FilesScanned int
	Findings     []Finding
	// Unparsable lists files that could not be read; they are neither migrated
	// nor pinned, because nothing can be said about them.
	Unparsable []string
}

// AffectedFiles returns the files with at least one finding, sorted.
func (r *Report) AffectedFiles() []string {
	seen := map[string]struct{}{}
	for _, f := range r.Findings {
		seen[f.File] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for f := range seen {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// FilesFor returns the files with a finding in the given area, sorted.
func (r *Report) FilesFor(area tryve.CompatArea) []string {
	seen := map[string]struct{}{}
	for _, f := range r.Findings {
		if f.Area == area {
			seen[f.File] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for f := range seen {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// Analyze inspects each file for behaviour that differs between the level it
// runs at now and the target level.
//
// Only differences in areas that are being adopted are reported: moving only
// `assertions` to v2 says nothing about interpolation.
func Analyze(paths []string, from, to tryve.CompatMode) (*Report, error) {
	report := &Report{}

	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			report.Unparsable = append(report.Unparsable, path)
			continue
		}

		var doc yaml.Node
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			report.Unparsable = append(report.Unparsable, path)
			continue
		}
		report.FilesScanned++

		root := documentRoot(&doc)
		if root == nil {
			continue
		}

		// A file that declares its own level is at that level, not the suite's.
		fileFrom := from
		if mode, ok := fileLevel(root); ok {
			fileFrom = mode
		}

		for _, area := range []tryve.CompatArea{
			tryve.CompatAssertions, tryve.CompatInterpolation,
			tryve.CompatExecution, tryve.CompatAdapters,
		} {
			// Only a transition from old to new behaviour can change anything.
			if fileFrom.Modern(area) || !to.Modern(area) {
				continue
			}
			report.Findings = append(report.Findings, analyzeArea(path, root, area)...)
		}
	}

	return report, nil
}

// analyzeArea dispatches to the checks for one area.
func analyzeArea(path string, root *yaml.Node, area tryve.CompatArea) []Finding {
	var findings []Finding
	forEachStep(root, func(step *yaml.Node) {
		switch area {
		case tryve.CompatAssertions:
			findings = append(findings, assertionFindings(path, step)...)
		case tryve.CompatInterpolation:
			findings = append(findings, interpolationFindings(path, step)...)
		case tryve.CompatExecution:
			findings = append(findings, executionFindings(path, step)...)
		case tryve.CompatAdapters:
			findings = append(findings, adapterFindings(path, step)...)
		}
	})
	return findings
}

// ---------------------------------------------------------------- helpers ---

// documentRoot unwraps a document node to its mapping.
func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	if doc.Kind == yaml.MappingNode {
		return doc
	}
	return nil
}

// mappingValue returns the value node for a key in a mapping, or nil.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// mappingKeys lists a mapping's keys in document order.
func mappingKeys(m *yaml.Node) []string {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	out := make([]string, 0, len(m.Content)/2)
	for i := 0; i+1 < len(m.Content); i += 2 {
		out = append(out, m.Content[i].Value)
	}
	return out
}

// fileLevel reads a file's own apiVersion and compatibility refinement.
// It reports false when the file declares neither.
func fileLevel(root *yaml.Node) (tryve.CompatMode, bool) {
	apiNode := mappingValue(root, "apiVersion")
	compatNode := mappingValue(root, "compatibility")
	if apiNode == nil && compatNode == nil {
		return tryve.LegacyCompat(), false
	}

	mode, err := tryve.ResolveLevel(decodeNode(apiNode), decodeNode(compatNode))
	if err != nil {
		return tryve.LegacyCompat(), false
	}
	return mode, true
}

// decodeNode decodes a YAML node to a plain value, or nil when absent.
func decodeNode(n *yaml.Node) any {
	if n == nil {
		return nil
	}
	var raw any
	if err := n.Decode(&raw); err != nil {
		return nil
	}
	return raw
}

// forEachStep visits every step in every phase of a test file.
func forEachStep(root *yaml.Node, visit func(step *yaml.Node)) {
	for _, phase := range []string{"setup", "execute", "verify", "teardown"} {
		seq := mappingValue(root, phase)
		if seq == nil || seq.Kind != yaml.SequenceNode {
			continue
		}
		for _, step := range seq.Content {
			if step.Kind == yaml.MappingNode {
				visit(step)
			}
		}
	}
}

// adapterOf returns a step's adapter name.
func adapterOf(step *yaml.Node) string {
	if v := mappingValue(step, "adapter"); v != nil {
		return v.Value
	}
	return ""
}

// actionOf returns a step's action name.
func actionOf(step *yaml.Node) string {
	if v := mappingValue(step, "action"); v != nil {
		return v.Value
	}
	return ""
}

// scalarStrings collects every scalar string under a node.
func scalarStrings(n *yaml.Node, out *[]*yaml.Node) {
	if n == nil {
		return
	}
	if n.Kind == yaml.ScalarNode && n.Tag == "!!str" {
		*out = append(*out, n)
		return
	}
	for _, c := range n.Content {
		scalarStrings(c, out)
	}
}

// describeNode renders a short, single-line excerpt of a node for a report.
func describeNode(n *yaml.Node) string {
	var raw any
	if err := n.Decode(&raw); err != nil {
		return ""
	}
	encoded, err := yaml.Marshal(raw)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(strings.ReplaceAll(string(encoded), "\n", " "))
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 70 {
		s = s[:70] + "…"
	}
	return s
}

// finding builds a Finding for a node.
func finding(path string, n *yaml.Node, area tryve.CompatArea, c Certainty, rule, detail string) Finding {
	line := 0
	if n != nil {
		line = n.Line
	}
	return Finding{File: path, Area: area, Certainty: c, Rule: rule, Detail: detail, Line: line}
}

// fmtDetail formats a detail string.
func fmtDetail(format string, args ...any) string { return fmt.Sprintf(format, args...) }

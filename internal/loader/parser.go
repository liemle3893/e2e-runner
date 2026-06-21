package loader

import (
	"fmt"
	"os"

	"github.com/liemle3893/go-tryve/internal/tryve"
	"gopkg.in/yaml.v3"
)

// knownStepFields is the set of keys that are handled as first-class fields on
// StepDefinition.  Every other key at the step level is collected into Params.
var knownStepFields = map[string]struct{}{
	"adapter":         {},
	"action":          {},
	"name":            {},
	"description":     {},
	"capture":         {},
	"assert":          {},
	"continueOnError": {},
	"retry":           {},
	"delay":           {},
	"id":              {},
}

// ParseFile reads the YAML test definition at path and returns a fully populated
// *tryve.TestDefinition.
//
// Adapter-specific fields (e.g. url, method, command, topic) sit at the top
// level of each step in the YAML format rather than under a "params" key.
// ParseFile collects those fields into StepDefinition.Params.
//
// Step IDs are assigned as "{phase}-{index}" (e.g. "execute-0").
// SourceFile is set to the resolved absolute path of the file.
func ParseFile(path string) (*tryve.TestDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, tryve.ConfigError(
			fmt.Sprintf("cannot read test file %q", path),
			"verify the file exists and is readable",
			err,
		)
	}

	// Unmarshal into typed struct for top-level fields.
	var td tryve.TestDefinition
	if err := yaml.Unmarshal(data, &td); err != nil {
		return nil, tryve.ConfigError(
			fmt.Sprintf("cannot parse test file %q", path),
			"verify the YAML syntax is valid",
			err,
		)
	}

	// Unmarshal into a raw map for full access to unknown step-level keys.
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, tryve.ConfigError(
			fmt.Sprintf("cannot parse test file %q (raw pass)", path),
			"verify the YAML syntax is valid",
			err,
		)
	}

	// Parse each phase from the raw map and enrich the typed struct.
	phases := []struct {
		name  string
		steps *[]tryve.StepDefinition
	}{
		{"setup", &td.Setup},
		{"execute", &td.Execute},
		{"verify", &td.Verify},
		{"teardown", &td.Teardown},
	}

	for _, ph := range phases {
		rawSteps, ok := raw[ph.name]
		if !ok {
			continue
		}
		stepList, ok := rawSteps.([]any)
		if !ok {
			continue
		}

		parsed := make([]tryve.StepDefinition, len(stepList))
		for i, rawStep := range stepList {
			stepMap, ok := rawStep.(map[string]any)
			if !ok {
				continue
			}
			s := parseStep(stepMap)
			s.ID = fmt.Sprintf("%s-%d", ph.name, i)
			parsed[i] = s
		}
		*ph.steps = parsed
	}

	td.SourceFile = path

	// If "retries" was not explicitly set in the YAML, use -1 to signal "use default".
	// This distinguishes "retries: 0" (no retries) from absent (use config default).
	if _, hasRetries := raw["retries"]; !hasRetries {
		td.Retries = -1
	}

	return &td, nil
}

// parseStep converts a raw YAML map for a single step into a StepDefinition.
// Known fields are set directly; all remaining keys are stored in Params.
func parseStep(m map[string]any) tryve.StepDefinition {
	var s tryve.StepDefinition
	s.Params = make(map[string]any)

	for k, v := range m {
		if _, isKnown := knownStepFields[k]; isKnown {
			switch k {
			case "adapter":
				s.Adapter, _ = v.(string)
			case "action":
				s.Action, _ = v.(string)
			case "name":
				s.Name, _ = v.(string)
			case "description":
				s.Description, _ = v.(string)
			case "continueOnError":
				s.ContinueOnError, _ = v.(bool)
			case "retry":
				s.Retry = toInt(v)
			case "delay":
				s.Delay = toInt(v)
			case "id":
				// id from YAML overrides the generated id only if non-empty.
				if id, ok := v.(string); ok && id != "" {
					s.ID = id
				}
			case "capture":
				s.Capture = toStringMap(v)
			case "assert":
				s.Assert = v
			}
		} else {
			s.Params[k] = v
		}
	}
	return s
}

// toStringMap converts a map[string]any to map[string]string by coercing each
// value with fmt.Sprintf.  Non-map inputs return nil.
func toStringMap(v any) map[string]string {
	raw, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, val := range raw {
		out[k] = fmt.Sprintf("%v", val)
	}
	return out
}

// toInt coerces common YAML numeric types to int.
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

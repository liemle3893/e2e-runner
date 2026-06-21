package loader

import (
	"fmt"

	"github.com/liemle3893/go-tryve/internal/tryve"
)

// validAdapters is the set of adapter names recognised by the runner.
var validAdapters = map[string]struct{}{
	"http":       {},
	"postgresql": {},
	"mongodb":    {},
	"redis":      {},
	"kafka":      {},
	"eventhub":   {},
	"shell":      {},
	"process":    {},
}

// validPriorities is the set of allowed priority strings.  An empty string is
// also acceptable (meaning "unset").
var validPriorities = map[string]struct{}{
	"":   {},
	"P0": {},
	"P1": {},
	"P2": {},
	"P3": {},
}

// Validate checks a TestDefinition for structural correctness and returns all
// discovered errors.  An empty slice means the definition is valid.
//
// Checks performed:
//   - name must be non-empty
//   - at least one execute step is required
//   - priority must be in the valid set
//   - timeout must be in [0, 300000]
//   - retries must be in [0, 5]
//   - every step must reference a valid adapter and specify an action
//   - per-adapter field requirements (url, command, sql, collection, topic, etc.)
func Validate(td *tryve.TestDefinition) []error {
	var errs []error

	if td.Name == "" {
		errs = append(errs, tryve.ValidationError(
			"test name is required",
			"add a 'name' field to the test file",
			nil,
		))
	}

	if len(td.Execute) == 0 {
		errs = append(errs, tryve.ValidationError(
			"at least one execute step is required",
			"add steps under the 'execute' phase",
			nil,
		))
	}

	if _, ok := validPriorities[string(td.Priority)]; !ok {
		errs = append(errs, tryve.ValidationError(
			fmt.Sprintf("invalid priority %q; must be one of P0, P1, P2, P3 or empty", td.Priority),
			"set priority to P0, P1, P2, or P3",
			nil,
		))
	}

	const maxTimeout = 300000
	if td.Timeout < 0 || td.Timeout > maxTimeout {
		errs = append(errs, tryve.ValidationError(
			fmt.Sprintf("timeout %d is out of range; must be between 0 and %d", td.Timeout, maxTimeout),
			"set timeout to a value between 0 and 300000 ms",
			nil,
		))
	}

	const maxRetries = 5
	if td.Retries < -1 || td.Retries > maxRetries {
		errs = append(errs, tryve.ValidationError(
			fmt.Sprintf("retries %d is out of range; must be between 0 and %d", td.Retries, maxRetries),
			"set retries to a value between 0 and 5",
			nil,
		))
	}

	allPhases := []struct {
		name  string
		steps []tryve.StepDefinition
	}{
		{"setup", td.Setup},
		{"execute", td.Execute},
		{"verify", td.Verify},
		{"teardown", td.Teardown},
	}

	seenNames := map[string]string{}
	for _, ph := range allPhases {
		for i, step := range ph.steps {
			stepRef := fmt.Sprintf("%s[%d]", ph.name, i)
			if step.ID != "" {
				stepRef = step.ID
			}
			errs = append(errs, validateStep(stepRef, &step)...)

			if step.Name != "" {
				if prev, dup := seenNames[step.Name]; dup {
					errs = append(errs, tryve.ValidationError(
						fmt.Sprintf("step %s: duplicate step name %q (first used at %s)", stepRef, step.Name, prev),
						"use unique step names to avoid capture collisions",
						nil,
					))
				} else {
					seenNames[step.Name] = stepRef
				}
			}
		}
	}

	return errs
}

// validateStep validates a single step and returns any errors found.
func validateStep(ref string, step *tryve.StepDefinition) []error {
	var errs []error

	if _, ok := validAdapters[step.Adapter]; !ok {
		errs = append(errs, tryve.ValidationError(
			fmt.Sprintf("step %s: unknown adapter %q", ref, step.Adapter),
			fmt.Sprintf("use one of: http, postgresql, mongodb, redis, kafka, eventhub, shell, process"),
			nil,
		))
		// No further per-adapter checks possible without a valid adapter.
		return errs
	}

	if step.Action == "" {
		errs = append(errs, tryve.ValidationError(
			fmt.Sprintf("step %s: action is required", ref),
			"set the 'action' field on the step",
			nil,
		))
	}

	errs = append(errs, validateAdapterConstraints(ref, step)...)
	return errs
}

// validateAdapterConstraints enforces per-adapter action and field rules.
func validateAdapterConstraints(ref string, step *tryve.StepDefinition) []error {
	var errs []error
	params := step.Params

	switch step.Adapter {
	case "http":
		errs = append(errs, requireAction(ref, step, "request")...)
		errs = append(errs, requireParam(ref, params, "url")...)

	case "shell":
		errs = append(errs, requireAction(ref, step, "exec")...)
		errs = append(errs, requireParam(ref, params, "command")...)

	case "postgresql":
		errs = append(errs, requireOneOfActions(ref, step, "execute", "query", "queryOne", "count")...)
		errs = append(errs, requireParam(ref, params, "sql")...)

	case "mongodb":
		errs = append(errs, requireOneOfActions(ref, step,
			"insertOne", "insertMany", "findOne", "find",
			"updateOne", "updateMany", "deleteOne", "deleteMany",
			"count", "aggregate")...)
		errs = append(errs, requireParam(ref, params, "collection")...)

	case "redis":
		errs = append(errs, requireOneOfActions(ref, step,
			"get", "set", "del", "exists", "incr",
			"hget", "hset", "hgetall", "keys", "flushPattern")...)

	case "kafka":
		errs = append(errs, requireOneOfActions(ref, step, "produce", "consume", "waitFor", "clear")...)
		if step.Action != "clear" {
			errs = append(errs, requireParam(ref, params, "topic")...)
		}

	case "eventhub":
		errs = append(errs, requireOneOfActions(ref, step, "publish", "waitFor", "consume", "clear")...)
		if step.Action != "clear" {
			errs = append(errs, requireParam(ref, params, "topic")...)
		}

	case "process":
		errs = append(errs, requireOneOfActions(ref, step, "start", "stop")...)
		if step.Action == "start" {
			errs = append(errs, requireParam(ref, params, "command")...)
			if bg, ok := params["background"]; ok {
				if bgBool, ok := bg.(bool); ok && !bgBool {
					errs = append(errs, tryve.ValidationError(
						fmt.Sprintf("step %s: background: false is not supported for process/start", ref),
						"remove background or set it to true",
						nil,
					))
				}
			}
		}
		if step.Action == "stop" {
			if !hasParam(params, "target") && !hasParam(params, "pid") {
				errs = append(errs, tryve.ValidationError(
					fmt.Sprintf("step %s: process/stop requires either 'target' or 'pid'", ref),
					"set 'target' to the process step name or 'pid' to a captured PID",
					nil,
				))
			}
		}
	}

	return errs
}

// hasParam reports whether the named key exists and is non-nil in params.
func hasParam(params map[string]any, key string) bool {
	if params == nil {
		return false
	}
	v, ok := params[key]
	return ok && v != nil
}

// requireAction returns an error when the step action does not match the single
// allowed action for the adapter.
func requireAction(ref string, step *tryve.StepDefinition, allowed string) []error {
	if step.Action != "" && step.Action != allowed {
		return []error{tryve.ValidationError(
			fmt.Sprintf("step %s: adapter %q only supports action %q, got %q",
				ref, step.Adapter, allowed, step.Action),
			fmt.Sprintf("set action to %q", allowed),
			nil,
		)}
	}
	return nil
}

// requireOneOfActions returns an error when the step action is not in the
// allowed set for the adapter.
func requireOneOfActions(ref string, step *tryve.StepDefinition, allowed ...string) []error {
	if step.Action == "" {
		return nil // caught by the generic "action required" check
	}
	for _, a := range allowed {
		if step.Action == a {
			return nil
		}
	}
	return []error{tryve.ValidationError(
		fmt.Sprintf("step %s: adapter %q does not support action %q", ref, step.Adapter, step.Action),
		fmt.Sprintf("valid actions for %s: %v", step.Adapter, allowed),
		nil,
	)}
}

// requireParam returns an error when the named key is absent or empty in params.
func requireParam(ref string, params map[string]any, key string) []error {
	if params == nil {
		return []error{tryve.ValidationError(
			fmt.Sprintf("step %s: required param %q is missing", ref, key),
			fmt.Sprintf("add %q to the step", key),
			nil,
		)}
	}
	v, ok := params[key]
	if !ok || v == nil || v == "" {
		return []error{tryve.ValidationError(
			fmt.Sprintf("step %s: required param %q is missing or empty", ref, key),
			fmt.Sprintf("set %q in the step", key),
			nil,
		)}
	}
	return nil
}

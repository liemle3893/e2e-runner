package reporter

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"sync"

	"github.com/liemle3893/go-tryve/internal/tryve"
)

// junitTestSuites is the root XML element for a JUnit report.
type junitTestSuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Time     string           `xml:"time,attr"`
	Suites   []junitTestSuite `xml:"testsuite"`
}

// junitTestSuite represents a single <testsuite> element.
type junitTestSuite struct {
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Skipped  int             `xml:"skipped,attr"`
	Time     string          `xml:"time,attr"`
	Cases    []junitTestCase `xml:"testcase"`
}

// junitTestCase represents a single <testcase> element.
type junitTestCase struct {
	Name    string        `xml:"name,attr"`
	Time    string        `xml:"time,attr,omitempty"`
	Failure *junitFailure `xml:"failure,omitempty"`
	Skipped *junitSkipped `xml:"skipped,omitempty"`
}

// junitFailure holds the <failure> element content and attributes.
type junitFailure struct {
	Message string `xml:"message,attr"`
	Content string `xml:",chardata"`
}

// junitSkipped holds the <skipped> element attributes.
type junitSkipped struct {
	Message string `xml:"message,attr,omitempty"`
}

// JUnit is a Reporter that accumulates test results in memory and writes a
// JUnit-compatible XML report to a file on Flush.
type JUnit struct {
	mu         sync.Mutex
	outputPath string
	results    []testEntry
}

// testEntry pairs a test definition with its final result for deferred XML rendering.
type testEntry struct {
	definition *tryve.TestDefinition
	result     *tryve.TestResult
}

// NewJUnit creates a JUnit reporter that will write its output to outputPath on Flush.
func NewJUnit(outputPath string) *JUnit {
	return &JUnit{outputPath: outputPath}
}

// OnSuiteStart satisfies the Reporter interface; no action is needed at suite start.
func (j *JUnit) OnSuiteStart(_ context.Context, _ *tryve.SuiteResult) error {
	return nil
}

// OnTestStart satisfies the Reporter interface; no action is needed at test start.
func (j *JUnit) OnTestStart(_ context.Context, _ *tryve.TestDefinition) error {
	return nil
}

// OnStepComplete satisfies the Reporter interface; step detail is captured via OnTestComplete.
func (j *JUnit) OnStepComplete(_ context.Context, _ *tryve.StepDefinition, _ *tryve.StepOutcome) error {
	return nil
}

// OnTestComplete accumulates the test result in memory for later XML rendering.
func (j *JUnit) OnTestComplete(_ context.Context, test *tryve.TestDefinition, result *tryve.TestResult) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.results = append(j.results, testEntry{definition: test, result: result})
	return nil
}

// OnSuiteComplete satisfies the Reporter interface; final counts are derived during Flush.
func (j *JUnit) OnSuiteComplete(_ context.Context, _ *tryve.SuiteResult, _ *tryve.SuiteResult) error {
	return nil
}

// Flush serialises all accumulated results to a JUnit XML file at the configured output path.
// It creates or truncates the file, writes a UTF-8 XML declaration followed by the
// <testsuites> document, and closes the file before returning.
func (j *JUnit) Flush() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	suite := j.buildSuite()
	suites := junitTestSuites{
		Tests:    suite.Tests,
		Failures: suite.Failures,
		Time:     suite.Time,
		Suites:   []junitTestSuite{suite},
	}

	f, err := os.Create(j.outputPath)
	if err != nil {
		return fmt.Errorf("junit reporter: create output file %q: %w", j.outputPath, err)
	}
	defer f.Close()

	if _, err := fmt.Fprint(f, xml.Header); err != nil {
		return fmt.Errorf("junit reporter: write XML header: %w", err)
	}

	enc := xml.NewEncoder(f)
	enc.Indent("", "  ")
	if err := enc.Encode(suites); err != nil {
		return fmt.Errorf("junit reporter: encode XML: %w", err)
	}
	return enc.Flush()
}

// buildSuite constructs the junitTestSuite from the accumulated results.
// Callers must hold j.mu.
func (j *JUnit) buildSuite() junitTestSuite {
	var totalSeconds float64
	var failures, skipped int
	cases := make([]junitTestCase, 0, len(j.results))

	for _, entry := range j.results {
		tc := buildTestCase(entry)
		switch entry.result.Status {
		case tryve.StatusFailed:
			failures++
		case tryve.StatusSkipped:
			skipped++
		}
		totalSeconds += entry.result.Duration.Seconds()
		cases = append(cases, tc)
	}

	return junitTestSuite{
		Name:     "tryve",
		Tests:    len(j.results),
		Failures: failures,
		Skipped:  skipped,
		Time:     formatSeconds(totalSeconds),
		Cases:    cases,
	}
}

// buildTestCase converts a single testEntry into a junitTestCase.
func buildTestCase(entry testEntry) junitTestCase {
	tc := junitTestCase{
		Name: entry.definition.Name,
		Time: formatSeconds(entry.result.Duration.Seconds()),
	}

	switch entry.result.Status {
	case tryve.StatusFailed:
		tc.Failure = buildFailure(entry.result)
	case tryve.StatusSkipped:
		tc.Skipped = &junitSkipped{Message: entry.definition.SkipReason}
		tc.Time = "" // skipped cases conventionally omit timing
	}

	return tc
}

// buildFailure constructs a junitFailure from the first failed step assertion,
// falling back to the test-level error when no step assertion is available.
func buildFailure(result *tryve.TestResult) *junitFailure {
	// Prefer the first failed step assertion for a precise message.
	for _, step := range result.Steps {
		if step.Status != tryve.StatusFailed {
			continue
		}
		for _, a := range step.Assertions {
			if !a.Passed {
				msg := fmt.Sprintf("assertion failed: %s", a.Message)
				body := fmt.Sprintf("Step %s: assertion failed at %s", step.Step.ID, a.Path)
				return &junitFailure{Message: msg, Content: body}
			}
		}
		// Step failed without a recorded assertion (e.g. adapter error).
		if step.Error != nil {
			msg := fmt.Sprintf("step error: %s", step.Error.Error())
			body := fmt.Sprintf("Step %s: %s", step.Step.ID, step.Error.Error())
			return &junitFailure{Message: msg, Content: body}
		}
	}

	// Fall back to the test-level error.
	if result.Error != nil {
		msg := fmt.Sprintf("test failed: %s", result.Error.Error())
		return &junitFailure{Message: msg, Content: result.Error.Error()}
	}

	return &junitFailure{Message: "test failed"}
}

// formatSeconds formats a duration in seconds as "S.SSS" for JUnit time attributes.
func formatSeconds(s float64) string {
	return fmt.Sprintf("%.3f", s)
}

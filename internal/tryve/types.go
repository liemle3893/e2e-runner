package tryve

import "time"

// TestPriority classifies a test's severity level.
type TestPriority string

const (
	PriorityP0 TestPriority = "P0"
	PriorityP1 TestPriority = "P1"
	PriorityP2 TestPriority = "P2"
	PriorityP3 TestPriority = "P3"
)

// TestStatus represents the outcome of a test or step execution.
type TestStatus string

const (
	StatusPassed  TestStatus = "passed"
	StatusFailed  TestStatus = "failed"
	StatusSkipped TestStatus = "skipped"
	StatusWarned  TestStatus = "warned"
)

// TestPhase identifies which lifecycle phase a step belongs to.
type TestPhase string

const (
	PhaseSetup    TestPhase = "setup"
	PhaseExecute  TestPhase = "execute"
	PhaseVerify   TestPhase = "verify"
	PhaseTeardown TestPhase = "teardown"
)

// TestDefinition holds the full parsed representation of a YAML test file.
type TestDefinition struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Priority    TestPriority      `yaml:"priority"`
	Tags        []string          `yaml:"tags"`
	Skip        bool              `yaml:"skip"`
	SkipReason  string            `yaml:"skipReason"`
	Timeout     int               `yaml:"timeout"`
	Retries     int               `yaml:"retries"`
	Depends     []string          `yaml:"depends"`
	Variables   map[string]any    `yaml:"variables"`
	Setup       []StepDefinition  `yaml:"setup"`
	Execute     []StepDefinition  `yaml:"execute"`
	Verify      []StepDefinition  `yaml:"verify"`
	Teardown    []StepDefinition  `yaml:"teardown"`
	// SourceFile holds the file path this definition was loaded from; not serialised.
	SourceFile string `yaml:"-"`
}

// StepDefinition describes a single adapter action within a test phase.
type StepDefinition struct {
	// ID is assigned at runtime and not present in YAML.
	ID              string            `yaml:"-"`
	// Name is an optional user-assigned label for this step; when set, captured
	// values are stored under a nested map keyed by this name (e.g. captured.<name>.port).
	Name            string            `yaml:"name"`
	Adapter         string            `yaml:"adapter"`
	Action          string            `yaml:"action"`
	Description     string            `yaml:"description"`
	// Params holds adapter-specific parameters; populated after YAML unmarshalling.
	Params          map[string]any    `yaml:"-"`
	Capture         map[string]string `yaml:"capture"`
	Assert          any               `yaml:"assert"`
	ContinueOnError bool              `yaml:"continueOnError"`
	Retry           int               `yaml:"retry"`
	Delay           int               `yaml:"delay"`
}

// StepResult carries the data returned by an adapter action execution.
type StepResult struct {
	// Data contains the primary output values produced by the step.
	Data     map[string]any
	Duration time.Duration
	Metadata map[string]any
}

// TestResult aggregates the overall outcome for a single test run.
type TestResult struct {
	Test       *TestDefinition
	Status     TestStatus
	Duration   time.Duration
	Steps      []StepOutcome
	Error      error
	RetryCount int
}

// StepOutcome records the execution result of one step within a test phase.
type StepOutcome struct {
	Step           *StepDefinition
	Phase          TestPhase
	Status         TestStatus
	Result         *StepResult
	ResolvedParams map[string]any
	Assertions     []AssertionOutcome
	Error          error
	Duration       time.Duration
}

// AssertionOutcome records the result of a single assertion check.
type AssertionOutcome struct {
	Path     string
	Operator string
	Expected any
	Actual   any
	Passed   bool
	Message  string
}

// SuiteResult aggregates the outcomes of all tests in a run.
type SuiteResult struct {
	Tests    []TestResult
	Duration time.Duration
	Passed   int
	Failed   int
	Skipped  int
	Total    int
}

// InterpolationContext holds the runtime variable state used during template interpolation.
type InterpolationContext struct {
	Variables map[string]any
	Captured  map[string]any
	BaseURL   string
	Env       map[string]string
}

// NewInterpolationContext initialises an InterpolationContext with empty maps ready for use.
func NewInterpolationContext() *InterpolationContext {
	return &InterpolationContext{
		Variables: make(map[string]any),
		Captured:  make(map[string]any),
		Env:       make(map[string]string),
	}
}

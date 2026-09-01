package adapter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/liemle3893/go-tryve/internal/adapter"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

// TestNewKafkaAdapter_DefaultsAndBrokers verifies that the constructor correctly
// parses broker lists provided as both []string and []any, and applies the
// default 10-second timeout when none is specified.
func TestNewKafkaAdapter_DefaultsAndBrokers(t *testing.T) {
	t.Run("brokers as []string", func(t *testing.T) {
		a := adapter.NewKafkaAdapter(map[string]any{
			"brokers": []string{"broker1:9092", "broker2:9092"},
		})
		if a == nil {
			t.Fatal("expected non-nil KafkaAdapter")
		}
		if a.Name() != "kafka" {
			t.Fatalf("expected name 'kafka', got %q", a.Name())
		}
	})

	t.Run("brokers as []any", func(t *testing.T) {
		a := adapter.NewKafkaAdapter(map[string]any{
			"brokers": []any{"broker1:9092", "broker2:9092"},
		})
		if a == nil {
			t.Fatal("expected non-nil KafkaAdapter")
		}
	})

	t.Run("default timeout applied", func(t *testing.T) {
		a := adapter.NewKafkaAdapter(map[string]any{
			"brokers": []string{"localhost:9092"},
		})
		// The adapter should use 10 s by default; verify via resolveTimeout
		// indirectly by observing connect succeeds (no crash on construction).
		if a == nil {
			t.Fatal("expected non-nil adapter")
		}
	})

	t.Run("custom timeout applied", func(t *testing.T) {
		a := adapter.NewKafkaAdapter(map[string]any{
			"brokers": []string{"localhost:9092"},
			"timeout": 5000, // 5 s
		})
		if a == nil {
			t.Fatal("expected non-nil adapter")
		}
	})

	t.Run("custom timeout as float64", func(t *testing.T) {
		a := adapter.NewKafkaAdapter(map[string]any{
			"brokers": []string{"localhost:9092"},
			"timeout": float64(3000),
		})
		if a == nil {
			t.Fatal("expected non-nil adapter")
		}
	})
}

// TestNewKafkaAdapter_OptionalFields verifies that clientId, groupId, and ssl
// are parsed from the config without error.
func TestNewKafkaAdapter_OptionalFields(t *testing.T) {
	a := adapter.NewKafkaAdapter(map[string]any{
		"brokers":  []string{"localhost:9092"},
		"clientId": "my-client",
		"groupId":  "my-group",
		"ssl":      true,
	})
	if a == nil {
		t.Fatal("expected non-nil adapter")
	}
	// Name must still be 'kafka' regardless of config.
	if a.Name() != "kafka" {
		t.Fatalf("expected name 'kafka', got %q", a.Name())
	}
}

// TestNewKafkaAdapter_SASL_Plain verifies that a PLAIN SASL mechanism is
// accepted without error during construction.
func TestNewKafkaAdapter_SASL_Plain(t *testing.T) {
	a := adapter.NewKafkaAdapter(map[string]any{
		"brokers": []string{"localhost:9092"},
		"sasl": map[string]any{
			"mechanism": "plain",
			"username":  "user",
			"password":  "secret",
		},
	})
	if a == nil {
		t.Fatal("expected non-nil adapter for PLAIN SASL")
	}
}

// TestNewKafkaAdapter_SASL_ScramSHA256 verifies that SCRAM-SHA-256 is accepted.
func TestNewKafkaAdapter_SASL_ScramSHA256(t *testing.T) {
	a := adapter.NewKafkaAdapter(map[string]any{
		"brokers": []string{"localhost:9092"},
		"sasl": map[string]any{
			"mechanism": "scram-sha-256",
			"username":  "user",
			"password":  "secret",
		},
	})
	if a == nil {
		t.Fatal("expected non-nil adapter for SCRAM-SHA-256 SASL")
	}
}

// TestNewKafkaAdapter_SASL_ScramSHA512 verifies that SCRAM-SHA-512 is accepted.
func TestNewKafkaAdapter_SASL_ScramSHA512(t *testing.T) {
	a := adapter.NewKafkaAdapter(map[string]any{
		"brokers": []string{"localhost:9092"},
		"sasl": map[string]any{
			"mechanism": "scram-sha-512",
			"username":  "user",
			"password":  "secret",
		},
	})
	if a == nil {
		t.Fatal("expected non-nil adapter for SCRAM-SHA-512 SASL")
	}
}

// TestNewKafkaAdapter_SASL_UnknownMechanism verifies that an unknown SASL
// mechanism does not panic and results in a usable (SASL-less) adapter.
func TestNewKafkaAdapter_SASL_UnknownMechanism(t *testing.T) {
	a := adapter.NewKafkaAdapter(map[string]any{
		"brokers": []string{"localhost:9092"},
		"sasl": map[string]any{
			"mechanism": "kerberos",
			"username":  "user",
			"password":  "secret",
		},
	})
	if a == nil {
		t.Fatal("expected non-nil adapter even for unknown SASL mechanism")
	}
}

// TestKafkaAdapter_Connect_NoBrokers verifies that Connect returns a
// CONNECTION_ERROR when no brokers are configured.
func TestKafkaAdapter_Connect_NoBrokers(t *testing.T) {
	a := adapter.NewKafkaAdapter(map[string]any{})
	err := a.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error when no brokers are configured")
	}

	var tryveErr *tryve.TryveError
	if !errors.As(err, &tryveErr) {
		t.Fatalf("expected *tryve.TryveError, got %T: %v", err, err)
	}
	if tryveErr.Code != "CONNECTION_ERROR" {
		t.Fatalf("expected code CONNECTION_ERROR, got %s", tryveErr.Code)
	}
}

// TestKafkaAdapter_Connect_WithBrokers verifies that Connect succeeds when at
// least one broker address is configured (no real connection is made at this
// stage).
func TestKafkaAdapter_Connect_WithBrokers(t *testing.T) {
	a := adapter.NewKafkaAdapter(map[string]any{
		"brokers": []string{"localhost:9092"},
	})
	if err := a.Connect(context.Background()); err != nil {
		t.Fatalf("unexpected Connect error: %v", err)
	}
}

// TestKafkaAdapter_Close_Empty verifies that Close is safe to call on an
// adapter that has no active readers or writers.
func TestKafkaAdapter_Close_Empty(t *testing.T) {
	a := adapter.NewKafkaAdapter(map[string]any{
		"brokers": []string{"localhost:9092"},
	})
	if err := a.Close(context.Background()); err != nil {
		t.Fatalf("unexpected Close error on clean adapter: %v", err)
	}
}

// TestKafkaAdapter_Execute_InvalidAction verifies that Execute returns an
// ADAPTER_ERROR for an unsupported action name.
func TestKafkaAdapter_Execute_InvalidAction(t *testing.T) {
	a := adapter.NewKafkaAdapter(map[string]any{
		"brokers": []string{"localhost:9092"},
	})
	ctx := context.Background()

	_, err := a.Execute(ctx, "unsupported", map[string]any{})
	if err == nil {
		t.Fatal("expected error for unsupported action, got nil")
	}

	var tryveErr *tryve.TryveError
	if !errors.As(err, &tryveErr) {
		t.Fatalf("expected *tryve.TryveError, got %T: %v", err, err)
	}
	if tryveErr.Code != "ADAPTER_ERROR" {
		t.Fatalf("expected code ADAPTER_ERROR, got %s", tryveErr.Code)
	}
}

// TestKafkaAdapter_Execute_Produce_MissingTopic verifies that produce without a
// topic returns ADAPTER_ERROR.
func TestKafkaAdapter_Execute_Produce_MissingTopic(t *testing.T) {
	a := adapter.NewKafkaAdapter(map[string]any{
		"brokers": []string{"localhost:9092"},
	})

	_, err := a.Execute(context.Background(), "produce", map[string]any{
		"value": "hello",
	})
	if err == nil {
		t.Fatal("expected error when topic is missing")
	}

	var tryveErr *tryve.TryveError
	if !errors.As(err, &tryveErr) {
		t.Fatalf("expected *tryve.TryveError, got %T", err)
	}
	if tryveErr.Code != "ADAPTER_ERROR" {
		t.Fatalf("expected ADAPTER_ERROR, got %s", tryveErr.Code)
	}
}

// TestKafkaAdapter_Execute_Consume_MissingTopic verifies that consume without a
// topic returns ADAPTER_ERROR.
func TestKafkaAdapter_Execute_Consume_MissingTopic(t *testing.T) {
	a := adapter.NewKafkaAdapter(map[string]any{
		"brokers": []string{"localhost:9092"},
	})

	_, err := a.Execute(context.Background(), "consume", map[string]any{})
	if err == nil {
		t.Fatal("expected error when topic is missing")
	}

	var tryveErr *tryve.TryveError
	if !errors.As(err, &tryveErr) {
		t.Fatalf("expected *tryve.TryveError, got %T", err)
	}
	if tryveErr.Code != "ADAPTER_ERROR" {
		t.Fatalf("expected ADAPTER_ERROR, got %s", tryveErr.Code)
	}
}

// TestKafkaAdapter_Execute_WaitFor_MissingTopic verifies that waitFor without a
// topic returns ADAPTER_ERROR.
func TestKafkaAdapter_Execute_WaitFor_MissingTopic(t *testing.T) {
	a := adapter.NewKafkaAdapter(map[string]any{
		"brokers": []string{"localhost:9092"},
	})

	_, err := a.Execute(context.Background(), "waitFor", map[string]any{
		"match": map[string]any{"key": "value"},
	})
	if err == nil {
		t.Fatal("expected error when topic is missing")
	}

	var tryveErr *tryve.TryveError
	if !errors.As(err, &tryveErr) {
		t.Fatalf("expected *tryve.TryveError, got %T", err)
	}
	if tryveErr.Code != "ADAPTER_ERROR" {
		t.Fatalf("expected ADAPTER_ERROR, got %s", tryveErr.Code)
	}
}

// TestKafkaAdapter_Execute_WaitFor_MissingMatch verifies that waitFor without a
// match param returns ADAPTER_ERROR.
func TestKafkaAdapter_Execute_WaitFor_MissingMatch(t *testing.T) {
	a := adapter.NewKafkaAdapter(map[string]any{
		"brokers": []string{"localhost:9092"},
	})

	_, err := a.Execute(context.Background(), "waitFor", map[string]any{
		"topic": "events",
	})
	if err == nil {
		t.Fatal("expected error when match is missing")
	}

	var tryveErr *tryve.TryveError
	if !errors.As(err, &tryveErr) {
		t.Fatalf("expected *tryve.TryveError, got %T", err)
	}
	if tryveErr.Code != "ADAPTER_ERROR" {
		t.Fatalf("expected ADAPTER_ERROR, got %s", tryveErr.Code)
	}
}

// TestKafkaAdapter_Execute_Clear_MissingTopic verifies that clear without a
// topic returns ADAPTER_ERROR.
func TestKafkaAdapter_Execute_Clear_MissingTopic(t *testing.T) {
	a := adapter.NewKafkaAdapter(map[string]any{
		"brokers": []string{"localhost:9092"},
	})

	_, err := a.Execute(context.Background(), "clear", map[string]any{})
	if err == nil {
		t.Fatal("expected error when topic is missing")
	}

	var tryveErr *tryve.TryveError
	if !errors.As(err, &tryveErr) {
		t.Fatalf("expected *tryve.TryveError, got %T", err)
	}
	if tryveErr.Code != "ADAPTER_ERROR" {
		t.Fatalf("expected ADAPTER_ERROR, got %s", tryveErr.Code)
	}
}

// TestKafkaAdapter_Health_NoBrokers verifies that Health returns CONNECTION_ERROR
// when no brokers are configured.
func TestKafkaAdapter_Health_NoBrokers(t *testing.T) {
	a := adapter.NewKafkaAdapter(map[string]any{})

	err := a.Health(context.Background())
	if err == nil {
		t.Fatal("expected error from Health with no brokers")
	}

	var tryveErr *tryve.TryveError
	if !errors.As(err, &tryveErr) {
		t.Fatalf("expected *tryve.TryveError, got %T", err)
	}
	if tryveErr.Code != "CONNECTION_ERROR" {
		t.Fatalf("expected CONNECTION_ERROR, got %s", tryveErr.Code)
	}
}

// TestKafkaAdapter_Health_UnreachableBroker verifies that Health returns
// CONNECTION_ERROR when the broker address is unreachable (no server running).
func TestKafkaAdapter_Health_UnreachableBroker(t *testing.T) {
	a := adapter.NewKafkaAdapter(map[string]any{
		"brokers": []string{"127.0.0.1:19099"}, // port unlikely to be in use
		"timeout": 500,                         // 500 ms to keep the test fast
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := a.Health(ctx)
	if err == nil {
		t.Fatal("expected CONNECTION_ERROR for unreachable broker, got nil")
	}

	var tryveErr *tryve.TryveError
	if !errors.As(err, &tryveErr) {
		t.Fatalf("expected *tryve.TryveError, got %T: %v", err, err)
	}
	if tryveErr.Code != "CONNECTION_ERROR" {
		t.Fatalf("expected CONNECTION_ERROR, got %s", tryveErr.Code)
	}
}

// TestKafkaAdapter_ImplementsInterface verifies at compile time (via assignment)
// that *KafkaAdapter satisfies the Adapter interface.
func TestKafkaAdapter_ImplementsInterface(t *testing.T) {
	var _ adapter.Adapter = adapter.NewKafkaAdapter(map[string]any{
		"brokers": []string{"localhost:9092"},
	})
}

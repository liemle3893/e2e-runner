package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs/v2"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

// EventHubAdapter publishes and consumes events on Azure Event Hubs.
// It maintains a single ProducerClient for the lifetime of the adapter;
// ConsumerClients are created on demand per consume/waitFor/clear operation
// because each must target a specific partition.
type EventHubAdapter struct {
	connectionString string
	eventHubName     string
	consumerGroup    string
	producer         *azeventhubs.ProducerClient
}

// NewEventHubAdapter constructs an EventHubAdapter from a raw config map.
// Recognised keys:
//
//   - connectionString (string, required): AMQP connection string for the Event Hub namespace.
//   - eventHubName (string, optional): the Event Hub entity name.
//     When omitted the name is read from the EntityPath in the connection string.
//   - consumerGroup (string, optional, default "$Default"): consumer group used for receive operations.
func NewEventHubAdapter(cfg map[string]any) *EventHubAdapter {
	connStr := getStrDefault(cfg, "connectionString", "")
	hubName := getStrDefault(cfg, "eventHubName", "")
	cg := getStrDefault(cfg, "consumerGroup", azeventhubs.DefaultConsumerGroup)
	if cg == "" {
		cg = azeventhubs.DefaultConsumerGroup
	}
	return &EventHubAdapter{
		connectionString: connStr,
		eventHubName:     hubName,
		consumerGroup:    cg,
	}
}

// Name returns the adapter's registered identifier.
func (a *EventHubAdapter) Name() string { return "eventhub" }

// Connect creates the ProducerClient used for all publish operations.
// It returns a ConnectionError when the connection string is missing or the
// ProducerClient cannot be initialised.
func (a *EventHubAdapter) Connect(_ context.Context) error {
	if a.connectionString == "" {
		return tryve.ConnectionError("eventhub",
			"connectionString is required in adapter configuration", nil)
	}
	if err := CheckUnresolvedEnvVars("eventhub", "connectionString", a.connectionString); err != nil {
		return err
	}
	producer, err := azeventhubs.NewProducerClientFromConnectionString(
		a.connectionString, a.eventHubName, nil,
	)
	if err != nil {
		return tryve.ConnectionError("eventhub",
			fmt.Sprintf("failed to create producer client: %v", err), err)
	}
	a.producer = producer
	return nil
}

// Close shuts down the ProducerClient.
func (a *EventHubAdapter) Close(ctx context.Context) error {
	if a.producer != nil {
		if err := a.producer.Close(ctx); err != nil {
			return tryve.ConnectionError("eventhub",
				fmt.Sprintf("failed to close producer client: %v", err), err)
		}
		a.producer = nil
	}
	return nil
}

// Health verifies that the producer client has been initialised.
// A nil producer means Connect was not called or it failed.
func (a *EventHubAdapter) Health(_ context.Context) error {
	if a.producer == nil {
		return tryve.ConnectionError("eventhub",
			"producer client is not initialised; call Connect first", nil)
	}
	return nil
}

// Execute dispatches the named action with the given parameters.
// Supported actions: "publish", "consume", "waitFor", "clear".
func (a *EventHubAdapter) Execute(ctx context.Context, action string, params map[string]any) (*tryve.StepResult, error) {
	switch action {
	case "publish":
		return a.publishAction(ctx, params)
	case "consume":
		return a.consumeAction(ctx, params)
	case "waitFor":
		return a.waitForAction(ctx, params)
	case "clear":
		return a.clearAction(ctx, params)
	default:
		return nil, tryve.AdapterError("eventhub", action,
			fmt.Sprintf("unsupported action %q; supported actions: publish, consume, waitFor, clear", action),
			nil,
		)
	}
}

// publishAction sends one or more events to an Event Hub.
// Params:
//   - topic / eventHubName (string): destination Event Hub name. Falls back to the
//     adapter's configured eventHubName when absent.
//   - body (string | map): event payload. Maps are JSON-encoded automatically.
//   - properties (map[string]any, optional): application properties attached to the event.
func (a *EventHubAdapter) publishAction(ctx context.Context, params map[string]any) (*tryve.StepResult, error) {
	hubName := resolveHubName(params, a.eventHubName)

	body, err := resolveBody(params)
	if err != nil {
		return nil, tryve.AdapterError("eventhub", "publish", err.Error(), err)
	}

	properties := getMap(params, "properties")

	ed := &azeventhubs.EventData{Body: body}
	if len(properties) > 0 {
		ed.Properties = properties
	}

	// When the action targets a different hub than the producer, create a transient
	// producer for this call. Otherwise reuse the existing one.
	producer := a.producer
	var transientProducer *azeventhubs.ProducerClient
	if hubName != "" && hubName != a.eventHubName {
		transientProducer, err = azeventhubs.NewProducerClientFromConnectionString(
			a.connectionString, hubName, nil,
		)
		if err != nil {
			return nil, tryve.AdapterError("eventhub", "publish",
				fmt.Sprintf("failed to create producer for hub %q: %v", hubName, err), err)
		}
		defer transientProducer.Close(ctx) //nolint:errcheck
		producer = transientProducer
	}

	if producer == nil {
		return nil, tryve.AdapterError("eventhub", "publish",
			"producer client is not initialised; call Connect first", nil)
	}

	var duration time.Duration
	duration, err = MeasureDuration(func() error {
		batch, batchErr := producer.NewEventDataBatch(ctx, nil)
		if batchErr != nil {
			return fmt.Errorf("create batch: %w", batchErr)
		}
		if addErr := batch.AddEventData(ed, nil); addErr != nil {
			return fmt.Errorf("add event to batch: %w", addErr)
		}
		return producer.SendEventDataBatch(ctx, batch, nil)
	})
	if err != nil {
		return nil, tryve.AdapterError("eventhub", "publish",
			fmt.Sprintf("failed to publish event: %v", err), err)
	}

	return SuccessResult(map[string]any{"ok": true}, duration, nil), nil
}

// consumeAction receives events from a single partition of an Event Hub.
// Params:
//   - topic (string): Event Hub name.
//   - timeout (int, ms): receive deadline in milliseconds.
//   - partitionId (string, optional): target partition. Defaults to "0".
func (a *EventHubAdapter) consumeAction(ctx context.Context, params map[string]any) (*tryve.StepResult, error) {
	hubName := resolveHubName(params, a.eventHubName)
	timeoutMs := getIntDefault(params, "timeout", 5000)
	partitionID := getStrDefault(params, "partitionId", "0")

	consumer, err := azeventhubs.NewConsumerClientFromConnectionString(
		a.connectionString, hubName, a.consumerGroup, nil,
	)
	if err != nil {
		return nil, tryve.AdapterError("eventhub", "consume",
			fmt.Sprintf("failed to create consumer client: %v", err), err)
	}
	defer consumer.Close(ctx) //nolint:errcheck

	receiveCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	events, duration, err := receiveFromPartition(receiveCtx, consumer, partitionID)
	if err != nil {
		return nil, tryve.AdapterError("eventhub", "consume",
			fmt.Sprintf("receive failed: %v", err), err)
	}

	serialised := serialiseEvents(events)
	return SuccessResult(map[string]any{
		"events": serialised,
		"count":  float64(len(serialised)),
	}, duration, nil), nil
}

// waitForAction consumes events until one matches the provided criteria or the
// timeout elapses.
// Params:
//   - topic (string): Event Hub name.
//   - timeout (int, ms): total deadline in milliseconds.
//   - filter (map[string]any): key-value pairs that must all be present in the
//     event's body (after JSON decoding) or application properties. "match" is
//     accepted as a synonym.
//   - partitionId (string, optional): target partition. Defaults to "0".
func (a *EventHubAdapter) waitForAction(ctx context.Context, params map[string]any) (*tryve.StepResult, error) {
	hubName := resolveHubName(params, a.eventHubName)
	timeoutMs := getIntDefault(params, "timeout", 10000)
	partitionID := getStrDefault(params, "partitionId", "0")
	match := matchCriteria(params)

	consumer, err := azeventhubs.NewConsumerClientFromConnectionString(
		a.connectionString, hubName, a.consumerGroup, nil,
	)
	if err != nil {
		return nil, tryve.AdapterError("eventhub", "waitFor",
			fmt.Sprintf("failed to create consumer client: %v", err), err)
	}
	defer consumer.Close(ctx) //nolint:errcheck

	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	start := time.Now()

	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		pollCtx, cancel := context.WithTimeout(ctx, remaining)
		events, _, pollErr := receiveFromPartition(pollCtx, consumer, partitionID)
		cancel()

		if pollErr != nil && !errors.Is(pollErr, context.DeadlineExceeded) {
			return nil, tryve.AdapterError("eventhub", "waitFor",
				fmt.Sprintf("receive error: %v", pollErr), pollErr)
		}

		for _, ev := range events {
			if eventMatchesMap(ev, match) {
				serialised := serialiseEvent(ev)
				return SuccessResult(serialised, time.Since(start), nil), nil
			}
		}
	}

	return nil, tryve.TimeoutError("waitFor",
		time.Duration(timeoutMs)*time.Millisecond)
}

// clearAction drains all currently available events from every partition.
// Params:
//   - topic (string): Event Hub name.
func (a *EventHubAdapter) clearAction(ctx context.Context, params map[string]any) (*tryve.StepResult, error) {
	hubName := resolveHubName(params, a.eventHubName)

	consumer, err := azeventhubs.NewConsumerClientFromConnectionString(
		a.connectionString, hubName, a.consumerGroup, nil,
	)
	if err != nil {
		return nil, tryve.AdapterError("eventhub", "clear",
			fmt.Sprintf("failed to create consumer client: %v", err), err)
	}
	defer consumer.Close(ctx) //nolint:errcheck

	props, err := consumer.GetEventHubProperties(ctx, nil)
	if err != nil {
		return nil, tryve.AdapterError("eventhub", "clear",
			fmt.Sprintf("failed to get event hub properties: %v", err), err)
	}

	total := 0
	start := time.Now()
	// Use a short per-partition drain window — just enough to flush what is there.
	const drainTimeout = 500 * time.Millisecond
	for _, pid := range props.PartitionIDs {
		drainCtx, cancel := context.WithTimeout(ctx, drainTimeout)
		events, _, _ := receiveFromPartition(drainCtx, consumer, pid)
		cancel()
		total += len(events)
	}

	return SuccessResult(map[string]any{"cleared": float64(total)}, time.Since(start), nil), nil
}

// --- helpers ---

// resolveHubName returns the Event Hub name from action params (keys "topic" or
// "eventHubName"), falling back to the adapter's configured name.
func resolveHubName(params map[string]any, fallback string) string {
	if n := getStrDefault(params, "topic", ""); n != "" {
		return n
	}
	if n := getStrDefault(params, "eventHubName", ""); n != "" {
		return n
	}
	return fallback
}

// resolveBody converts the "body" param to a []byte payload.
// Strings are used as-is; maps/structs are JSON-encoded.
func resolveBody(params map[string]any) ([]byte, error) {
	raw, ok := params["body"]
	if !ok || raw == nil {
		return []byte{}, nil
	}
	switch v := raw.(type) {
	case string:
		return []byte(v), nil
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		return encoded, nil
	}
}

// receiveFromPartition opens a PartitionClient starting from the latest offset
// and collects all events within the context deadline.
func receiveFromPartition(ctx context.Context, consumer *azeventhubs.ConsumerClient, partitionID string) ([]*azeventhubs.ReceivedEventData, time.Duration, error) {
	pc, err := consumer.NewPartitionClient(partitionID, &azeventhubs.PartitionClientOptions{
		StartPosition: azeventhubs.StartPosition{
			Latest: ptrBool(true),
		},
	})
	if err != nil {
		return nil, 0, fmt.Errorf("create partition client: %w", err)
	}
	defer pc.Close(ctx) //nolint:errcheck

	var events []*azeventhubs.ReceivedEventData
	duration, err := MeasureDuration(func() error {
		var receiveErr error
		events, receiveErr = pc.ReceiveEvents(ctx, 100, nil)
		if errors.Is(receiveErr, context.DeadlineExceeded) {
			// A deadline just means no more events; not a hard failure.
			return nil
		}
		return receiveErr
	})
	return events, duration, err
}

// serialiseEvents converts a slice of ReceivedEventData to a []any suitable
// for inclusion in StepResult.Data.
func serialiseEvents(events []*azeventhubs.ReceivedEventData) []any {
	out := make([]any, 0, len(events))
	for _, ev := range events {
		out = append(out, serialiseEvent(ev))
	}
	return out
}

// serialiseEvent converts a single ReceivedEventData to a map[string]any.
// The body is decoded as a UTF-8 string; if it is valid JSON it is further
// decoded into a native Go value.
func serialiseEvent(ev *azeventhubs.ReceivedEventData) map[string]any {
	bodyStr := string(ev.Body)
	var bodyVal any = bodyStr
	var decoded any
	if json.Unmarshal(ev.Body, &decoded) == nil {
		bodyVal = decoded
	}

	m := map[string]any{
		"body":           bodyVal,
		"sequenceNumber": float64(ev.SequenceNumber),
		"offset":         ev.Offset,
		"properties":     ev.Properties,
	}
	if ev.EnqueuedTime != nil {
		m["enqueuedTime"] = ev.EnqueuedTime.UTC().Format(time.RFC3339)
	}
	if ev.PartitionKey != nil {
		m["partitionKey"] = *ev.PartitionKey
	}
	return m
}

// eventMatchesMap checks whether ev satisfies all key-value pairs in criteria.
// It inspects the event's application Properties map and, when the body is a
// JSON object, the decoded body fields.
func eventMatchesMap(ev *azeventhubs.ReceivedEventData, criteria map[string]any) bool {
	if len(criteria) == 0 {
		return true
	}

	// Build a lookup combining body fields and application properties.
	lookup := make(map[string]any)
	for k, v := range ev.Properties {
		lookup[k] = v
	}
	var bodyMap map[string]any
	if json.Unmarshal(ev.Body, &bodyMap) == nil {
		for k, v := range bodyMap {
			lookup[k] = v
		}
	}

	for k, want := range criteria {
		got, ok := lookup[k]
		if !ok {
			return false
		}
		if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
			return false
		}
	}
	return true
}

// getIntDefault retrieves an integer param from params, supporting both int and
// float64 (the default JSON number type). Returns defaultVal when the key is
// absent or the value cannot be converted.
func getIntDefault(params map[string]any, key string, defaultVal int) int {
	v, ok := params[key]
	if !ok || v == nil {
		return defaultVal
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return defaultVal
	}
}

// ptrBool returns a pointer to the given bool value.
func ptrBool(b bool) *bool { return &b }

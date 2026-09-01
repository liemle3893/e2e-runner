package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	kafka "github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"

	"github.com/liemle3893/go-tryve/internal/tryve"
)

// KafkaAdapter provides produce, consume, waitFor, and clear actions against
// a Kafka cluster using the segmentio/kafka-go library.
type KafkaAdapter struct {
	brokers   []string
	clientID  string
	groupID   string
	timeout   time.Duration
	mechanism sasl.Mechanism // nil when SASL is not configured
	tls       bool

	compat tryve.CompatMode

	mu      sync.Mutex
	readers []*kafka.Reader
	writers []*kafka.Writer
}

// NewKafkaAdapter constructs a KafkaAdapter from a generic config map.
//
// Recognised keys:
//   - brokers    ([]string or []any) — required; list of host:port addresses
//   - clientId   (string)            — optional Kafka client identifier
//   - groupId    (string)            — optional consumer group ID
//   - timeout    (int, ms)           — per-operation default timeout (default 10 000)
//   - ssl        (bool)              — reserved; TLS flag (not yet wired to dialer)
//   - sasl       (map)               — optional SASL config with keys:
//     mechanism (plain|scram-sha-256|scram-sha-512), username, password
func NewKafkaAdapter(cfg map[string]any) *KafkaAdapter {
	return NewKafkaAdapterWithCompat(cfg, tryve.LegacyCompat())
}

// NewKafkaAdapterWithCompat is NewKafkaAdapter with an explicit compatibility
// mode selecting whether a produce may create a missing topic.
func NewKafkaAdapterWithCompat(cfg map[string]any, mode tryve.CompatMode) *KafkaAdapter {
	a := &KafkaAdapter{
		timeout: 10 * time.Second,
		compat:  mode,
	}

	// -- brokers ----------------------------------------------------------------
	if v, ok := cfg["brokers"]; ok {
		switch bv := v.(type) {
		case []string:
			a.brokers = append(a.brokers, bv...)
		case []any:
			for _, b := range bv {
				if s, ok := b.(string); ok {
					a.brokers = append(a.brokers, s)
				}
			}
		}
	}

	// -- optional string fields -------------------------------------------------
	a.clientID = getStrDefault(cfg, "clientId", "")
	a.groupID = getStrDefault(cfg, "groupId", "")

	// -- timeout ----------------------------------------------------------------
	if v, ok := cfg["timeout"]; ok {
		switch tv := v.(type) {
		case int:
			if tv > 0 {
				a.timeout = time.Duration(tv) * time.Millisecond
			}
		case float64:
			if tv > 0 {
				a.timeout = time.Duration(int(tv)) * time.Millisecond
			}
		}
	}

	// -- ssl --------------------------------------------------------------------
	if v, ok := cfg["ssl"]; ok {
		if b, ok := v.(bool); ok {
			a.tls = b
		}
	}

	// -- sasl -------------------------------------------------------------------
	if v, ok := cfg["sasl"]; ok {
		if saslMap, ok := v.(map[string]any); ok {
			mech, _ := saslMap["mechanism"].(string)
			user, _ := saslMap["username"].(string)
			pass, _ := saslMap["password"].(string)
			if m := buildSASLMechanism(mech, user, pass); m != nil {
				a.mechanism = m
			}
		}
	}

	return a
}

// buildSASLMechanism constructs the appropriate sasl.Mechanism for the given
// mechanism name. Returns nil for unrecognised or empty mechanism names.
func buildSASLMechanism(mechanism, username, password string) sasl.Mechanism {
	switch strings.ToLower(mechanism) {
	case "plain":
		return plain.Mechanism{Username: username, Password: password}
	case "scram-sha-256":
		m, err := scram.Mechanism(scram.SHA256, username, password)
		if err != nil {
			return nil
		}
		return m
	case "scram-sha-512":
		m, err := scram.Mechanism(scram.SHA512, username, password)
		if err != nil {
			return nil
		}
		return m
	default:
		return nil
	}
}

// Name returns the adapter's registered identifier.
func (a *KafkaAdapter) Name() string { return "kafka" }

// Connect validates that at least one broker is configured. The actual TCP
// connection is established lazily when a reader/writer is created, so this
// method only checks preconditions.
func (a *KafkaAdapter) Connect(_ context.Context) error {
	if len(a.brokers) == 0 {
		return tryve.ConnectionError("kafka", "no brokers configured", nil)
	}
	return nil
}

// Close closes all active readers and writers created during Execute calls.
func (a *KafkaAdapter) Close(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	var lastErr error
	for _, r := range a.readers {
		if err := r.Close(); err != nil {
			lastErr = err
		}
	}
	for _, w := range a.writers {
		if err := w.Close(); err != nil {
			lastErr = err
		}
	}
	a.readers = nil
	a.writers = nil
	return lastErr
}

// Health dials the first broker to verify TCP connectivity.
func (a *KafkaAdapter) Health(ctx context.Context) error {
	if len(a.brokers) == 0 {
		return tryve.ConnectionError("kafka", "no brokers configured", nil)
	}

	d := a.newDialer()
	conn, err := d.DialContext(ctx, "tcp", a.brokers[0])
	if err != nil {
		return tryve.ConnectionError("kafka", fmt.Sprintf("health check failed: %v", err), err)
	}
	_ = conn.Close()
	return nil
}

// Execute dispatches the named action with the given parameters.
//
// Supported actions: produce, consume, waitFor, clear.
func (a *KafkaAdapter) Execute(ctx context.Context, action string, params map[string]any) (*tryve.StepResult, error) {
	switch action {
	case "produce":
		return a.executeProduce(ctx, params)
	case "consume":
		return a.executeConsume(ctx, params)
	case "waitFor":
		return a.executeWaitFor(ctx, params)
	case "clear":
		return a.executeClear(ctx, params)
	default:
		return nil, tryve.AdapterError("kafka", action,
			fmt.Sprintf("unsupported action %q: supported actions are produce, consume, waitFor, clear", action), nil)
	}
}

// --------------------------------------------------------------------------
// Action implementations
// --------------------------------------------------------------------------

// executeProduce writes one or more messages to the specified topic.
//
// Required params: topic, plus the payload in one of three forms:
//
//	message:  {key, value, headers, partition}   — a single message
//	messages: [{key, value, …}, …]               — a batch
//	value: …                                     — the payload alone
//
// The nested forms are what the documentation shows; the flat form is the
// original spelling. All three are accepted, because a test written against the
// documentation previously produced an empty payload in silence.
func (a *KafkaAdapter) executeProduce(ctx context.Context, params map[string]any) (*tryve.StepResult, error) {
	topic := getStrDefault(params, "topic", "")
	if topic == "" {
		return nil, tryve.AdapterError("kafka", "produce", "missing required param: topic", nil)
	}

	specs, err := produceSpecs(params)
	if err != nil {
		return nil, tryve.AdapterError("kafka", "produce", err.Error(), err)
	}

	messages := make([]kafka.Message, 0, len(specs))
	for i, spec := range specs {
		msg, buildErr := buildMessage(spec)
		if buildErr != nil {
			return nil, tryve.AdapterError("kafka", "produce",
				fmt.Sprintf("message %d: %v", i, buildErr), buildErr)
		}
		messages = append(messages, msg)
	}

	w := a.newWriter(topic, tryve.CompatOrDefault(ctx, a.compat).Modern(tryve.CompatAdapters))
	a.trackWriter(w)
	defer func() {
		_ = w.Close()
		a.untrackWriter(w)
	}()

	duration, err := MeasureDuration(func() error {
		return w.WriteMessages(ctx, messages...)
	})
	if err != nil {
		return nil, tryve.AdapterError("kafka", "produce", "failed to write message", err)
	}

	return SuccessResult(map[string]any{
		"ok":    true,
		"count": float64(len(messages)),
	}, duration, nil), nil
}

// produceSpecs normalises the accepted payload spellings into a list of message
// definitions.
func produceSpecs(params map[string]any) ([]map[string]any, error) {
	if raw, ok := params["messages"]; ok && raw != nil {
		items, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("param \"messages\" must be a list, got %T", raw)
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("param \"messages\" must not be empty")
		}
		specs := make([]map[string]any, 0, len(items))
		for i, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("messages[%d] must be an object, got %T", i, item)
			}
			specs = append(specs, m)
		}
		return specs, nil
	}

	if raw, ok := params["message"]; ok && raw != nil {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("param \"message\" must be an object, got %T", raw)
		}
		return []map[string]any{m}, nil
	}

	if _, ok := params["value"]; ok {
		return []map[string]any{params}, nil
	}

	return nil, fmt.Errorf("missing payload: set \"message\", \"messages\", or \"value\"")
}

// buildMessage converts one message definition into a kafka.Message.
//
// The topic is deliberately left unset: it belongs to the Writer, and kafka-go
// rejects a write where both carry one.
func buildMessage(spec map[string]any) (kafka.Message, error) {
	value, ok := spec["value"]
	if !ok {
		return kafka.Message{}, fmt.Errorf("missing required field: value")
	}

	encoded, err := encodeValue(value)
	if err != nil {
		return kafka.Message{}, fmt.Errorf("failed to encode value: %w", err)
	}

	msg := kafka.Message{Value: encoded}

	if k := getStrDefault(spec, "key", ""); k != "" {
		msg.Key = []byte(k)
	}
	if p, ok := spec["partition"]; ok {
		msg.Partition = int(toFloat(p))
	}
	if hv, ok := spec["headers"].(map[string]any); ok {
		for k, v := range hv {
			msg.Headers = append(msg.Headers, kafka.Header{
				Key:   k,
				Value: []byte(fmt.Sprintf("%v", v)),
			})
		}
	}
	return msg, nil
}

// toFloat coerces a YAML numeric value to float64.
func toFloat(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	}
	return 0
}

// executeConsume reads one message from the specified topic/group.
//
// Required params: topic.
// Optional params: timeout (int, ms) — overrides adapter-level timeout.
func (a *KafkaAdapter) executeConsume(ctx context.Context, params map[string]any) (*tryve.StepResult, error) {
	topic := getStrDefault(params, "topic", "")
	if topic == "" {
		return nil, tryve.AdapterError("kafka", "consume", "missing required param: topic", nil)
	}

	opTimeout := a.resolveTimeout(params)
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()

	r := a.newReader(topic)
	a.trackReader(r)
	defer func() {
		_ = r.Close()
		a.untrackReader(r)
	}()

	var msg kafka.Message
	var duration time.Duration
	var err error

	duration, err = MeasureDuration(func() error {
		msg, err = r.ReadMessage(ctx)
		return err
	})
	if err != nil {
		return nil, tryve.AdapterError("kafka", "consume", "failed to read message", err)
	}

	return SuccessResult(messageToData(msg), duration, nil), nil
}

// executeWaitFor reads messages until one matches all conditions in the match
// map or the operation times out.
//
// Required params: topic, and the match criteria as either "filter" (as
// documented) or "match" (the original spelling; both are accepted).
// Optional params: timeout (int, ms).
func (a *KafkaAdapter) executeWaitFor(ctx context.Context, params map[string]any) (*tryve.StepResult, error) {
	topic := getStrDefault(params, "topic", "")
	if topic == "" {
		return nil, tryve.AdapterError("kafka", "waitFor", "missing required param: topic", nil)
	}

	matchMap := matchCriteria(params)
	if len(matchMap) == 0 {
		return nil, tryve.AdapterError("kafka", "waitFor",
			"missing or invalid required param: filter (a map of field/value pairs to match)", nil)
	}

	opTimeout := a.resolveTimeout(params)
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()

	r := a.newReader(topic)
	a.trackReader(r)
	defer func() {
		_ = r.Close()
		a.untrackReader(r)
	}()

	for {
		var msg kafka.Message
		var err error
		_, err = MeasureDuration(func() error {
			msg, err = r.ReadMessage(ctx)
			return err
		})
		if err != nil {
			if ctx.Err() != nil {
				return nil, tryve.TimeoutError("kafka.waitFor", opTimeout)
			}
			return nil, tryve.AdapterError("kafka", "waitFor", "failed to read message", err)
		}

		if messageMatches(msg, matchMap) {
			var duration time.Duration
			return SuccessResult(matchedMessageData(msg), duration, nil), nil
		}
	}
}

// executeClear drains all pending messages from the topic and returns the
// number consumed.
//
// Required params: topic.
// Optional params: timeout (int, ms).
func (a *KafkaAdapter) executeClear(ctx context.Context, params map[string]any) (*tryve.StepResult, error) {
	topic := getStrDefault(params, "topic", "")
	if topic == "" {
		return nil, tryve.AdapterError("kafka", "clear", "missing required param: topic", nil)
	}

	opTimeout := a.resolveTimeout(params)
	drainCtx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()

	r := a.newReader(topic)
	a.trackReader(r)
	defer func() {
		_ = r.Close()
		a.untrackReader(r)
	}()

	cleared := 0
	start := time.Now()

	for {
		_, err := r.ReadMessage(drainCtx)
		if err != nil {
			// Timeout or context cancellation means the queue is drained.
			break
		}
		cleared++
	}

	duration := time.Since(start)
	return SuccessResult(map[string]any{"cleared": float64(cleared)}, duration, nil), nil
}

// --------------------------------------------------------------------------
// Helpers — reader / writer construction
// --------------------------------------------------------------------------

// newDialer constructs a kafka.Dialer with SASL configured if a mechanism is set.
func (a *KafkaAdapter) newDialer() *kafka.Dialer {
	d := &kafka.Dialer{
		Timeout:   a.timeout,
		DualStack: true,
	}
	if a.mechanism != nil {
		d.SASLMechanism = a.mechanism
	}
	if a.clientID != "" {
		d.ClientID = a.clientID
	}
	return d
}

// newWriter creates a kafka.Writer for the given topic.
func (a *KafkaAdapter) newWriter(topic string, autoCreateTopic bool) *kafka.Writer {
	transport := &kafka.Transport{
		Dial: (&net.Dialer{
			Timeout: a.timeout,
		}).DialContext,
	}
	if a.mechanism != nil {
		transport.SASL = a.mechanism
	}
	w := &kafka.Writer{
		Addr:      kafka.TCP(a.brokers...),
		Topic:     topic,
		Transport: transport,
		// Tests routinely produce to a topic that does not exist yet. Without
		// this the first produce fails with "Unknown Topic Or Partition" and the
		// test author has to provision the topic out of band. Creating topics is
		// a behaviour change, so it follows the adapters area.
		AllowAutoTopicCreation: autoCreateTopic,
	}
	if a.clientID != "" {
		// kafka.Writer does not expose ClientID directly; set via Balancer metadata.
		_ = a.clientID
	}
	return w
}

// newReader creates a kafka.Reader for the given topic.
func (a *KafkaAdapter) newReader(topic string) *kafka.Reader {
	cfg := kafka.ReaderConfig{
		Brokers:  a.brokers,
		Topic:    topic,
		MinBytes: 1,
		MaxBytes: 10e6, // 10 MB
		MaxWait:  a.timeout,
	}
	if a.groupID != "" {
		cfg.GroupID = a.groupID
	}
	if a.mechanism != nil {
		cfg.Dialer = a.newDialer()
	}
	return kafka.NewReader(cfg)
}

// --------------------------------------------------------------------------
// Helpers — reader / writer lifecycle tracking
// --------------------------------------------------------------------------

func (a *KafkaAdapter) trackReader(r *kafka.Reader) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.readers = append(a.readers, r)
}

func (a *KafkaAdapter) untrackReader(r *kafka.Reader) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, rr := range a.readers {
		if rr == r {
			a.readers = append(a.readers[:i], a.readers[i+1:]...)
			return
		}
	}
}

func (a *KafkaAdapter) trackWriter(w *kafka.Writer) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.writers = append(a.writers, w)
}

func (a *KafkaAdapter) untrackWriter(w *kafka.Writer) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, ww := range a.writers {
		if ww == w {
			a.writers = append(a.writers[:i], a.writers[i+1:]...)
			return
		}
	}
}

// --------------------------------------------------------------------------
// Helpers — message conversion and matching
// --------------------------------------------------------------------------

// encodeValue converts a value param to []byte.
// Strings are encoded directly; maps and other types are JSON-marshalled.
func encodeValue(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	switch tv := v.(type) {
	case string:
		return []byte(tv), nil
	default:
		return json.Marshal(tv)
	}
}

// messageToData converts a kafka.Message to the canonical result map.
func messageToData(msg kafka.Message) map[string]any {
	headers := make(map[string]any, len(msg.Headers))
	for _, h := range msg.Headers {
		headers[h.Key] = string(h.Value)
	}

	// Attempt JSON decode of value; fall back to raw string.
	var value any = string(msg.Value)
	var decoded any
	if err := json.Unmarshal(msg.Value, &decoded); err == nil {
		value = decoded
	}

	return map[string]any{
		"key":       string(msg.Key),
		"value":     value,
		"headers":   headers,
		"topic":     msg.Topic,
		"partition": float64(msg.Partition),
		"offset":    float64(msg.Offset),
	}
}

// messageMatches returns true when every key/value pair in matchMap is satisfied
// by the message.
//
// Filter keys address the decoded message body with dot-notation
// ("type", "data.userId"), which is what a producer's payload looks like to the
// test author. Envelope fields (key, topic, partition, offset, headers) are also
// matchable, so a filter can select on either.
//
// Matching the envelope's top level alone — as this once did — meant a filter on
// a body field never matched anything, and waitFor simply waited out its timeout.
func messageMatches(msg kafka.Message, matchMap map[string]any) bool {
	data := messageToData(msg)
	body := data["value"]

	for k, want := range matchMap {
		got, ok := lookupDotted(body, k)
		if !ok {
			// Fall back to the envelope for key/topic/partition/offset/headers.
			got, ok = lookupDotted(data, k)
		}
		if !ok {
			return false
		}
		if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
			return false
		}
	}
	return true
}

// lookupDotted walks a dot-separated path through nested maps and slices.
func lookupDotted(root any, path string) (any, bool) {
	cur := root
	for _, seg := range strings.Split(path, ".") {
		switch typed := cur.(type) {
		case map[string]any:
			v, ok := typed[seg]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(typed) {
				return nil, false
			}
			cur = typed[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

// matchedMessageData shapes the result of a successful waitFor.
//
// The decoded payload's fields sit at the top level so `path: "data.message"`
// and `capture: {id: "data.id"}` address the payload directly, matching how
// filters are written.
//
// The envelope fields (key, value, headers, topic, partition, offset) are kept
// alongside them, so an existing assertion on `$.value.type` or `$.key` keeps
// working. Payload fields win a name collision, since those are what the
// documented paths refer to; the untouched envelope is always available under
// "message".
func matchedMessageData(msg kafka.Message) map[string]any {
	envelope := messageToData(msg)

	body, ok := envelope["value"].(map[string]any)
	if !ok {
		// A non-object payload (a bare string or number) has no fields to lift.
		return envelope
	}

	data := make(map[string]any, len(envelope)+len(body)+1)
	for k, v := range envelope {
		data[k] = v
	}
	data["message"] = envelope
	for k, v := range body {
		data[k] = v
	}
	return data
}

// resolveTimeout extracts an optional "timeout" param (ms) from params,
// falling back to the adapter-level default.
func (a *KafkaAdapter) resolveTimeout(params map[string]any) time.Duration {
	if v, ok := params["timeout"]; ok {
		switch tv := v.(type) {
		case int:
			if tv > 0 {
				return time.Duration(tv) * time.Millisecond
			}
		case float64:
			if tv > 0 {
				return time.Duration(int(tv)) * time.Millisecond
			}
		}
	}
	return a.timeout
}

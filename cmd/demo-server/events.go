package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs/v2"
)

func (s *services) handlePublishEvent(w http.ResponseWriter, r *http.Request) {
	if s.ehProducer == nil {
		writeError(w, http.StatusInternalServerError, "Failed to publish event")
		return
	}

	var req struct {
		Type         string         `json:"type"`
		Data         map[string]any `json:"data"`
		PartitionKey string         `json:"partitionKey"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Type == "" || req.Data == nil {
		writeError(w, http.StatusBadRequest, "type and data are required")
		return
	}

	event := map[string]any{
		"type":      req.Type,
		"data":      req.Data,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(event)
	if err != nil {
		log.Printf("error publishing event: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to publish event")
		return
	}

	batchOpts := &azeventhubs.EventDataBatchOptions{}
	if req.PartitionKey != "" {
		batchOpts.PartitionKey = &req.PartitionKey
	}

	batch, err := s.ehProducer.NewEventDataBatch(r.Context(), batchOpts)
	if err != nil {
		log.Printf("error publishing event: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to publish event")
		return
	}

	ed := &azeventhubs.EventData{
		Body: body,
		Properties: map[string]any{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}
	if err := batch.AddEventData(ed, nil); err != nil {
		log.Printf("error publishing event: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to publish event")
		return
	}
	if err := s.ehProducer.SendEventDataBatch(r.Context(), batch, nil); err != nil {
		log.Printf("error publishing event: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to publish event")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"message": "Event published successfully",
		"event":   event,
	})
}

func (s *services) handleConsumeEvents(w http.ResponseWriter, r *http.Request) {
	timeoutMs := queryInt(r, "timeout", 5000)
	maxEvents := queryInt(r, "maxEvents", 10)

	consumer, err := azeventhubs.NewConsumerClientFromConnectionString(
		eventHubConnString(), eventHubName(), eventHubConsumerGroup(), nil)
	if err != nil {
		log.Printf("error consuming events: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to consume events")
		return
	}
	defer consumer.Close(context.Background()) //nolint:errcheck

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// Read every partition, not just partition 0. A hub with more than one
	// partition spreads events across them, so a single-partition read reports
	// an empty result for events that were published successfully.
	props, err := consumer.GetEventHubProperties(ctx, nil)
	if err != nil {
		log.Printf("error consuming events: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to consume events")
		return
	}

	type partitionResult struct {
		events []*azeventhubs.ReceivedEventData
		err    error
	}
	results := make(chan partitionResult, len(props.PartitionIDs))

	for _, id := range props.PartitionIDs {
		go func(partitionID string) {
			results <- partitionResult{events: receivePartition(ctx, consumer, partitionID, maxEvents)}
		}(id)
	}

	events := make([]map[string]any, 0, maxEvents)
	for range props.PartitionIDs {
		res := <-results
		for _, ev := range res.events {
			if len(events) >= maxEvents {
				break
			}
			events = append(events, serialiseEvent(ev))
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"count":  len(events),
		"events": events,
	})
}

// receivePartition reads up to maxEvents from one partition, returning what
// arrived before the context deadline. A deadline is the normal end of a bounded
// poll, not a failure, so it yields the events collected so far.
func receivePartition(ctx context.Context, consumer *azeventhubs.ConsumerClient,
	partitionID string, maxEvents int) []*azeventhubs.ReceivedEventData {

	partition, err := consumer.NewPartitionClient(partitionID, &azeventhubs.PartitionClientOptions{
		StartPosition: azeventhubs.StartPosition{Latest: boolPtr(true)},
	})
	if err != nil {
		log.Printf("consume: partition %s: %v", partitionID, err)
		return nil
	}
	defer partition.Close(context.Background()) //nolint:errcheck

	received, err := partition.ReceiveEvents(ctx, maxEvents, nil)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		log.Printf("consume: partition %s: %v", partitionID, err)
	}
	return received
}

// serialiseEvent renders a received event for the JSON response, decoding the
// body when it is JSON so assertions can address its fields directly.
func serialiseEvent(ev *azeventhubs.ReceivedEventData) map[string]any {
	var body any = string(ev.Body)
	var decoded any
	if json.Unmarshal(ev.Body, &decoded) == nil {
		body = decoded
	}

	out := map[string]any{
		"body":           body,
		"properties":     ev.Properties,
		"offset":         ev.Offset,
		"sequenceNumber": ev.SequenceNumber,
	}
	if ev.EnqueuedTime != nil {
		out["enqueuedTimeUtc"] = ev.EnqueuedTime.UTC().Format(time.RFC3339)
	}
	if ev.PartitionKey != nil {
		out["partitionKey"] = *ev.PartitionKey
	}
	return out
}

// queryInt reads an integer query parameter, falling back to a default when it
// is absent or unparseable.
func queryInt(r *http.Request, name string, fallback int) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// boolPtr returns a pointer to b.
func boolPtr(b bool) *bool { return &b }

package adapter

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/liemle3893/go-tryve/internal/tryve"
)

// MongoDBAdapter executes MongoDB operations against a target database.
// It maintains a persistent mongo.Client across operations within the same
// adapter instance. Connect must be called before Execute or Health.
type MongoDBAdapter struct {
	connStr  string
	dbName   string
	client   *mongo.Client
	database *mongo.Database
	compat   tryve.CompatMode
}

// NewMongoDBAdapter constructs a MongoDBAdapter from a config map.
// Required keys: "connectionString" (string), "database" (string).
// Missing or non-string values default to empty string.
func NewMongoDBAdapter(cfg map[string]any) *MongoDBAdapter {
	return NewMongoDBAdapterWithCompat(cfg, tryve.LegacyCompat())
}

// NewMongoDBAdapterWithCompat is NewMongoDBAdapter with an explicit
// compatibility mode selecting the findOne result shape.
func NewMongoDBAdapterWithCompat(cfg map[string]any, mode tryve.CompatMode) *MongoDBAdapter {
	connStr, _ := cfg["connectionString"].(string)
	dbName, _ := cfg["database"].(string)
	return &MongoDBAdapter{
		connStr: connStr,
		dbName:  dbName,
		compat:  mode,
	}
}

// Name returns the adapter's registered identifier.
func (a *MongoDBAdapter) Name() string { return "mongodb" }

// Connect establishes the MongoDB client and resolves the target database handle.
// It returns a ConnectionError when the URI is invalid or the driver fails to initialise.
func (a *MongoDBAdapter) Connect(_ context.Context) error {
	if a.connStr == "" {
		return tryve.ConnectionError("mongodb", "connectionString must not be empty", nil)
	}
	if err := CheckUnresolvedEnvVars("mongodb", "connectionString", a.connStr); err != nil {
		return err
	}

	client, err := mongo.Connect(options.Client().ApplyURI(a.connStr))
	if err != nil {
		return tryve.ConnectionError("mongodb", "failed to connect", err)
	}
	a.client = client
	a.database = client.Database(a.dbName)
	return nil
}

// Close disconnects the MongoDB client and releases all held resources.
func (a *MongoDBAdapter) Close(ctx context.Context) error {
	if a.client != nil {
		if err := a.client.Disconnect(ctx); err != nil {
			return tryve.ConnectionError("mongodb", "failed to disconnect", err)
		}
	}
	return nil
}

// Health performs a lightweight ping to verify the server is reachable.
func (a *MongoDBAdapter) Health(ctx context.Context) error {
	if err := a.client.Ping(ctx, nil); err != nil {
		return tryve.ConnectionError("mongodb", "health check failed", err)
	}
	return nil
}

// Execute dispatches the named action with the provided parameters.
// All actions require a "collection" string parameter.
// Supported actions: insertOne, insertMany, findOne, find, updateOne, updateMany,
// deleteOne, deleteMany, count, aggregate.
func (a *MongoDBAdapter) Execute(ctx context.Context, action string, params map[string]any) (*tryve.StepResult, error) {
	collName, err := getStr(params, "collection")
	if err != nil {
		return nil, tryve.AdapterError("mongodb", action, "missing required param: collection", err)
	}
	coll := a.database.Collection(collName)

	switch action {
	case "insertOne":
		return a.insertOne(ctx, coll, params)
	case "insertMany":
		return a.insertMany(ctx, coll, params)
	case "findOne":
		return a.findOne(ctx, coll, params)
	case "find":
		return a.find(ctx, coll, params)
	case "updateOne":
		return a.updateOne(ctx, coll, params)
	case "updateMany":
		return a.updateMany(ctx, coll, params)
	case "deleteOne":
		return a.deleteOne(ctx, coll, params)
	case "deleteMany":
		return a.deleteMany(ctx, coll, params)
	case "count":
		return a.count(ctx, coll, params)
	case "aggregate":
		return a.aggregate(ctx, coll, params)
	default:
		return nil, tryve.AdapterError(
			"mongodb", action,
			fmt.Sprintf("unsupported action %q", action),
			nil,
		)
	}
}

// insertOne inserts a single document into the collection.
// Params: "document" (map[string]any, required).
// Returns: {"insertedId": id}.
func (a *MongoDBAdapter) insertOne(ctx context.Context, coll *mongo.Collection, params map[string]any) (*tryve.StepResult, error) {
	doc, ok := params["document"]
	if !ok || doc == nil {
		return nil, tryve.AdapterError("mongodb", "insertOne", "missing required param: document", nil)
	}

	var result *mongo.InsertOneResult
	duration, err := MeasureDuration(func() error {
		var doErr error
		result, doErr = coll.InsertOne(ctx, doc)
		return doErr
	})
	if err != nil {
		return nil, tryve.AdapterError("mongodb", "insertOne", "operation failed", err)
	}

	data := map[string]any{
		"insertedId": result.InsertedID,
	}
	return SuccessResult(data, duration, nil), nil
}

// insertMany inserts multiple documents into the collection.
// Params: "documents" ([]any or []map[string]any, required).
// Returns: {"insertedIds": []any}.
func (a *MongoDBAdapter) insertMany(ctx context.Context, coll *mongo.Collection, params map[string]any) (*tryve.StepResult, error) {
	raw, ok := params["documents"]
	if !ok || raw == nil {
		return nil, tryve.AdapterError("mongodb", "insertMany", "missing required param: documents", nil)
	}

	docs, err := toSliceOfAny(raw)
	if err != nil {
		return nil, tryve.AdapterError("mongodb", "insertMany", "param \"documents\" must be a slice", err)
	}

	var result *mongo.InsertManyResult
	duration, opErr := MeasureDuration(func() error {
		var doErr error
		result, doErr = coll.InsertMany(ctx, docs)
		return doErr
	})
	if opErr != nil {
		return nil, tryve.AdapterError("mongodb", "insertMany", "operation failed", opErr)
	}

	data := map[string]any{
		"insertedIds": result.InsertedIDs,
	}
	return SuccessResult(data, duration, nil), nil
}

// findOne retrieves a single document matching the filter.
//
// Params:
//   - "filter"     (map[string]any, defaults to an empty filter)
//   - "allowEmpty" (bool) — treat "no match" as a value to assert on rather than
//     a step error.
//
// The document's fields are returned at the top level, so `path: "email"` and
// `capture: {id: "_id"}` address them directly — the same shape the postgresql
// adapter's queryOne returns. A "document" key carrying the whole document is
// also present for callers that prefer to address it explicitly.
func (a *MongoDBAdapter) findOne(ctx context.Context, coll *mongo.Collection, params map[string]any) (*tryve.StepResult, error) {
	filter := filterParam(params)

	var doc map[string]any
	duration, err := MeasureDuration(func() error {
		return coll.FindOne(ctx, filter).Decode(&doc)
	})

	if errors.Is(err, mongo.ErrNoDocuments) {
		if boolParam(params, "allowEmpty") {
			return SuccessResult(map[string]any{"found": false}, duration, nil), nil
		}
		return nil, tryve.AdapterError("mongodb", "findOne",
			"no document matched the filter; set allowEmpty: true to treat that as a value to assert on", err)
	}
	if err != nil {
		return nil, tryve.AdapterError("mongodb", "findOne", "operation failed", err)
	}

	// Before the adapters area changed, findOne returned only {"document": …};
	// the fields were not addressable at the top level.
	if !tryve.CompatOrDefault(ctx, a.compat).Modern(tryve.CompatAdapters) {
		return SuccessResult(map[string]any{"document": doc}, duration, nil), nil
	}

	data := make(map[string]any, len(doc)+2)
	for k, v := range doc {
		data[k] = v
	}
	// Only fill in the helper keys when the document does not already use those
	// field names, so a real field is never shadowed.
	if _, taken := data["document"]; !taken {
		data["document"] = doc
	}
	if _, taken := data["found"]; !taken {
		data["found"] = true
	}
	return SuccessResult(data, duration, nil), nil
}

// find retrieves all documents matching the filter.
// Params: "filter" (map[string]any, defaults to empty filter).
// Returns: {"documents": []map[string]any, "count": float64}.
func (a *MongoDBAdapter) find(ctx context.Context, coll *mongo.Collection, params map[string]any) (*tryve.StepResult, error) {
	filter := filterParam(params)

	var docs []map[string]any
	duration, err := MeasureDuration(func() error {
		cursor, doErr := coll.Find(ctx, filter)
		if doErr != nil {
			return doErr
		}
		return cursor.All(ctx, &docs)
	})
	if err != nil {
		return nil, tryve.AdapterError("mongodb", "find", "operation failed", err)
	}

	if docs == nil {
		docs = []map[string]any{}
	}
	data := map[string]any{
		"documents": docs,
		"count":     float64(len(docs)),
	}
	return SuccessResult(data, duration, nil), nil
}

// updateOne updates the first document matching the filter.
// Params: "filter" (map[string]any), "update" (map[string]any, required).
// Returns: {"matchedCount": float64, "modifiedCount": float64}.
func (a *MongoDBAdapter) updateOne(ctx context.Context, coll *mongo.Collection, params map[string]any) (*tryve.StepResult, error) {
	filter := filterParam(params)
	update, ok := params["update"]
	if !ok || update == nil {
		return nil, tryve.AdapterError("mongodb", "updateOne", "missing required param: update", nil)
	}

	var result *mongo.UpdateResult
	duration, err := MeasureDuration(func() error {
		var doErr error
		result, doErr = coll.UpdateOne(ctx, filter, update)
		return doErr
	})
	if err != nil {
		return nil, tryve.AdapterError("mongodb", "updateOne", "operation failed", err)
	}

	data := map[string]any{
		"matchedCount":  float64(result.MatchedCount),
		"modifiedCount": float64(result.ModifiedCount),
	}
	return SuccessResult(data, duration, nil), nil
}

// updateMany updates all documents matching the filter.
// Params: "filter" (map[string]any), "update" (map[string]any, required).
// Returns: {"matchedCount": float64, "modifiedCount": float64}.
func (a *MongoDBAdapter) updateMany(ctx context.Context, coll *mongo.Collection, params map[string]any) (*tryve.StepResult, error) {
	filter := filterParam(params)
	update, ok := params["update"]
	if !ok || update == nil {
		return nil, tryve.AdapterError("mongodb", "updateMany", "missing required param: update", nil)
	}

	var result *mongo.UpdateResult
	duration, err := MeasureDuration(func() error {
		var doErr error
		result, doErr = coll.UpdateMany(ctx, filter, update)
		return doErr
	})
	if err != nil {
		return nil, tryve.AdapterError("mongodb", "updateMany", "operation failed", err)
	}

	data := map[string]any{
		"matchedCount":  float64(result.MatchedCount),
		"modifiedCount": float64(result.ModifiedCount),
	}
	return SuccessResult(data, duration, nil), nil
}

// deleteOne removes the first document matching the filter.
// Params: "filter" (map[string]any, defaults to empty filter).
// Returns: {"deletedCount": float64}.
func (a *MongoDBAdapter) deleteOne(ctx context.Context, coll *mongo.Collection, params map[string]any) (*tryve.StepResult, error) {
	filter := filterParam(params)

	var result *mongo.DeleteResult
	duration, err := MeasureDuration(func() error {
		var doErr error
		result, doErr = coll.DeleteOne(ctx, filter)
		return doErr
	})
	if err != nil {
		return nil, tryve.AdapterError("mongodb", "deleteOne", "operation failed", err)
	}

	data := map[string]any{
		"deletedCount": float64(result.DeletedCount),
	}
	return SuccessResult(data, duration, nil), nil
}

// deleteMany removes all documents matching the filter.
// Params: "filter" (map[string]any, defaults to empty filter).
// Returns: {"deletedCount": float64}.
func (a *MongoDBAdapter) deleteMany(ctx context.Context, coll *mongo.Collection, params map[string]any) (*tryve.StepResult, error) {
	filter := filterParam(params)

	var result *mongo.DeleteResult
	duration, err := MeasureDuration(func() error {
		var doErr error
		result, doErr = coll.DeleteMany(ctx, filter)
		return doErr
	})
	if err != nil {
		return nil, tryve.AdapterError("mongodb", "deleteMany", "operation failed", err)
	}

	data := map[string]any{
		"deletedCount": float64(result.DeletedCount),
	}
	return SuccessResult(data, duration, nil), nil
}

// count returns the number of documents matching the filter.
// Params: "filter" (map[string]any, defaults to empty filter).
// Returns: {"count": float64}.
func (a *MongoDBAdapter) count(ctx context.Context, coll *mongo.Collection, params map[string]any) (*tryve.StepResult, error) {
	filter := filterParam(params)

	var n int64
	duration, err := MeasureDuration(func() error {
		var doErr error
		n, doErr = coll.CountDocuments(ctx, filter)
		return doErr
	})
	if err != nil {
		return nil, tryve.AdapterError("mongodb", "count", "operation failed", err)
	}

	data := map[string]any{
		"count": float64(n),
	}
	return SuccessResult(data, duration, nil), nil
}

// aggregate executes an aggregation pipeline against the collection.
// Params: "pipeline" ([]any or []map[string]any, required).
// Returns: {"documents": []map[string]any}.
func (a *MongoDBAdapter) aggregate(ctx context.Context, coll *mongo.Collection, params map[string]any) (*tryve.StepResult, error) {
	raw, ok := params["pipeline"]
	if !ok || raw == nil {
		return nil, tryve.AdapterError("mongodb", "aggregate", "missing required param: pipeline", nil)
	}

	pipeline, err := toSliceOfAny(raw)
	if err != nil {
		return nil, tryve.AdapterError("mongodb", "aggregate", "param \"pipeline\" must be a slice", err)
	}

	var docs []map[string]any
	duration, opErr := MeasureDuration(func() error {
		cursor, doErr := coll.Aggregate(ctx, pipeline)
		if doErr != nil {
			return doErr
		}
		return cursor.All(ctx, &docs)
	})
	if opErr != nil {
		return nil, tryve.AdapterError("mongodb", "aggregate", "operation failed", opErr)
	}

	if docs == nil {
		docs = []map[string]any{}
	}
	data := map[string]any{
		"documents": docs,
	}
	return SuccessResult(data, duration, nil), nil
}

// filterParam extracts the "filter" key from params as a map[string]any.
// When absent or not a map, it returns an empty document (match-all).
func filterParam(params map[string]any) map[string]any {
	v, ok := params["filter"]
	if !ok || v == nil {
		return map[string]any{}
	}
	m, ok := v.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return m
}

// toSliceOfAny converts a value to []any.
// Accepts []any directly, or []map[string]any by converting each element.
func toSliceOfAny(v any) ([]any, error) {
	switch t := v.(type) {
	case []any:
		return t, nil
	case []map[string]any:
		out := make([]any, len(t))
		for i, m := range t {
			out[i] = m
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected []any or []map[string]any, got %T", v)
	}
}

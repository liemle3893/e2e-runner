package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// healthTimeout bounds every backend health probe, so an unreachable service
// makes /health slow by one timeout rather than hanging it.
const healthTimeout = 5 * time.Second

// services holds the backend clients the handlers use. A nil client means that
// backend failed to initialise; handlers and health checks treat it as down
// rather than panicking.
type services struct {
	pg         *pgxpool.Pool
	redis      *redis.Client
	mongo      *mongo.Client
	mongoDB    *mongo.Database
	ehProducer *azeventhubs.ProducerClient
}

// newServices returns an empty set of services ready for init.
func newServices() *services { return &services{} }

// env reads an environment variable, falling back to a default.
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// eventHubName is the hub the demo publishes to and consumes from.
func eventHubName() string { return env("EVENTHUB_NAME", "fortune-events") }

// eventHubConnString returns the Event Hubs connection string, defaulting to the
// local emulator that docker-compose starts.
func eventHubConnString() string {
	return env("EVENTHUB_CONNECTION_STRING",
		"Endpoint=sb://localhost;SharedAccessKeyName=RootManageSharedAccessKey;"+
			"SharedAccessKey=SAS_KEY_VALUE;UseDevelopmentEmulator=true;")
}

// eventHubConsumerGroup returns the consumer group used by /events/consume.
func eventHubConsumerGroup() string { return env("EVENTHUB_CONSUMER_GROUP", "$Default") }

// init connects every backend, reporting failures without aborting startup.
func (s *services) init(ctx context.Context) {
	log.Println("initializing services…")

	if pool, err := pgxpool.New(ctx, env("POSTGRES_CONNECTION_STRING",
		"postgresql://fortune:fortune@localhost:5432/fortune_play")); err != nil {
		log.Printf("postgres initialization failed: %v", err)
	} else {
		s.pg = pool
		log.Println("postgres initialized")
	}

	if opts, err := redis.ParseURL(env("REDIS_CONNECTION_STRING", "redis://localhost:6379")); err != nil {
		log.Printf("redis initialization failed: %v", err)
	} else {
		s.redis = redis.NewClient(opts)
		log.Println("redis initialized")
	}

	if client, err := mongo.Connect(options.Client().ApplyURI(
		env("MONGODB_CONNECTION_STRING", "mongodb://root:root@localhost:27017"))); err != nil {
		log.Printf("mongodb initialization failed: %v", err)
	} else {
		s.mongo = client
		s.mongoDB = client.Database(env("MONGODB_DATABASE", "demo"))
		log.Println("mongodb initialized")
	}

	if producer, err := azeventhubs.NewProducerClientFromConnectionString(
		eventHubConnString(), eventHubName(), nil); err != nil {
		log.Printf("eventhub initialization failed: %v", err)
	} else {
		s.ehProducer = producer
		log.Println("eventhub initialized")
	}
}

// close releases every backend connection.
func (s *services) close(ctx context.Context) {
	if s.pg != nil {
		s.pg.Close()
	}
	if s.redis != nil {
		_ = s.redis.Close()
	}
	if s.mongo != nil {
		_ = s.mongo.Disconnect(ctx)
	}
	if s.ehProducer != nil {
		_ = s.ehProducer.Close(ctx)
	}
}

// pgHealthy reports whether PostgreSQL answers a ping.
func (s *services) pgHealthy(ctx context.Context) bool {
	if s.pg == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	if err := s.pg.Ping(ctx); err != nil {
		log.Printf("postgres health check failed: %v", err)
		return false
	}
	return true
}

// redisHealthy reports whether Redis answers PING.
func (s *services) redisHealthy(ctx context.Context) bool {
	if s.redis == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	if err := s.redis.Ping(ctx).Err(); err != nil {
		log.Printf("redis health check failed: %v", err)
		return false
	}
	return true
}

// mongoHealthy reports whether MongoDB answers a ping.
func (s *services) mongoHealthy(ctx context.Context) bool {
	if s.mongo == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	if err := s.mongo.Ping(ctx, nil); err != nil {
		log.Printf("mongodb health check failed: %v", err)
		return false
	}
	return true
}

// eventHubHealthy reports whether the Event Hub's properties can be read.
func (s *services) eventHubHealthy(ctx context.Context) bool {
	if s.ehProducer == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	props, err := s.ehProducer.GetEventHubProperties(ctx, nil)
	if err != nil {
		log.Printf("eventhub health check failed: %v", err)
		return false
	}
	return props.Name != ""
}

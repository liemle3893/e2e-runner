VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build demo-server test test-v lint clean run

build:
	go build -ldflags "$(LDFLAGS)" -o bin/tryve ./cmd/tryve

# demo-server is the system under test for tests/e2e/. It is not shipped;
# build it only when running the E2E suite against docker-compose services.
demo-server:
	go build -o bin/demo-server ./cmd/demo-server

test:
	go test ./...

test-v:
	go test -v ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

run:
	go run ./cmd/tryve $(ARGS)

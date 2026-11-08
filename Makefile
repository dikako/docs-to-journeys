.PHONY: build test vet fmt check clean

build: ## Build the CLI into bin/journeys
	go build -o bin/journeys ./cmd/journeys

test: ## Run unit tests (no device or Claude calls)
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal

check: vet test ## What CI runs
	@test -z "$$(gofmt -l cmd internal)" || (echo "run make fmt"; gofmt -l cmd internal; exit 1)

clean:
	rm -rf bin

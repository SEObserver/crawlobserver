.PHONY: build run test clean migrate

BINARY=seocrawler

build:
	go build -o $(BINARY) ./cmd/seocrawler

run: build
	./$(BINARY)

test:
	go test ./... -v -race

test-cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -f $(BINARY) coverage.out coverage.html

migrate: build
	./$(BINARY) migrate

lint:
	golangci-lint run ./...

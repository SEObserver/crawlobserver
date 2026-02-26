.PHONY: build run test clean migrate frontend

VERSION ?= dev
LDFLAGS = -ldflags "-X github.com/SEObserver/seocrawler/internal/updater.Version=$(VERSION)"
BINARY = seocrawler

frontend:
	cd frontend && npm install && npm run build
	rm -rf internal/server/frontend/dist
	mkdir -p internal/server/frontend
	cp -r frontend/dist internal/server/frontend/

build: frontend
	go build $(LDFLAGS) -o $(BINARY) ./cmd/seocrawler

build-go:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/seocrawler

run: build
	./$(BINARY)

test:
	go test ./... -v -race

test-cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -f $(BINARY) coverage.out coverage.html
	rm -rf internal/server/frontend/dist

migrate: build-go
	./$(BINARY) migrate

lint:
	golangci-lint run ./...

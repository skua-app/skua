.PHONY: build run test vet lint fmt check clean coverage

BIN := bin/server

build:
	go build -ldflags="-s -w" -trimpath -o $(BIN) ./cmd/server

run:
	go run ./cmd/server

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -l -w .

check: fmt vet lint test

clean:
	rm -rf bin/

coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out | grep total

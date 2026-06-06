.PHONY: run test test-race vet check

run:
	go run ./cmd/goclaw

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

check: test test-race vet

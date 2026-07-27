.PHONY: run test vet build

run:
	go run ./cmd/waypoint-dev

test:
	go test -race ./...

vet:
	go vet ./...

build:
	go build -o bin/waypoint-dev ./cmd/waypoint-dev

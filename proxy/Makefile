.PHONY: run test build clean

run:
	@set -a && . ./.env && set +a && go run ./cmd/waypoint-proxy

test:
	go test ./...

build:
	go build -o bin/waypoint-proxy ./cmd/waypoint-proxy

clean:
	rm -rf bin/ dist/

.PHONY: run test vet build

run:
	WAYPOINT_AUDIT_DB=./.local/audit.db \
	WAYPOINT_ALERTS_DB=./.local/alerts.db \
	WAYPOINT_LOCAL_ACCOUNT_SEED=./.local/local-account.nk \
	go run ./cmd/waypoint-agent -identity ./.local/identity.toml -http :8080

test:
	go test -race ./...

vet:
	go vet ./...

build:
	go build -o bin/waypoint-agent ./cmd/waypoint-agent

module github.com/waypointos/waypoint/sim

go 1.25.0

replace github.com/waypointos/waypoint/protocol/gen/go => ../protocol/gen/go

require (
	github.com/foxglove/mcap/go/mcap v1.9.0
	github.com/nats-io/nats.go v1.52.0
	github.com/stretchr/testify v1.11.1
	github.com/waypointos/waypoint/protocol/gen/go v0.0.0-00010101000000-000000000000
	github.com/waypointos/waypoint/protocol/platform v0.0.0-00010101000000-000000000000
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.15.0 // indirect
	golang.org/x/crypto v0.49.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/waypointos/waypoint/protocol/platform => ../protocol/platform

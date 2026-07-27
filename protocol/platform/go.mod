module github.com/waypointos/waypoint/protocol/platform

go 1.25.0

replace github.com/waypointos/waypoint/protocol/gen/go => ../gen/go

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/stretchr/testify v1.11.1
	github.com/waypointos/waypoint/protocol/gen/go v0.0.0-00010101000000-000000000000
	golang.org/x/sys v0.46.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

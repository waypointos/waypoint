// manifest.go — emits a JSON manifest from module.toml + version + sha256.
// Used by release CI to publish a JSON sidecar alongside the .raw.
package main

import (
	"encoding/json"
	"flag"
	"os"

	"github.com/BurntSushi/toml"
)

func main() {
	tomlPath := flag.String("toml", "", "path to module.toml")
	version := flag.String("version", "", "version string")
	sha256 := flag.String("sha256", "", "sha256 hex of the .raw")
	flag.Parse()
	var raw map[string]any
	if _, err := toml.DecodeFile(*tomlPath, &raw); err != nil {
		panic(err)
	}
	raw["version"] = *version
	raw["sha256"] = *sha256
	if err := json.NewEncoder(os.Stdout).Encode(raw); err != nil {
		panic(err)
	}
}

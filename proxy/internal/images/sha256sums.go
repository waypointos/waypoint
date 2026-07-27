// Package images catalogs OS image (.swu) releases for the Releases feature.
package images

import "strings"

// ParseSha256Sums parses a standard `sha256sum` file ("<hex>  <filename>" per
// line) into a filename -> hex map. Missing keys return "".
func ParseSha256Sums(data []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			name := strings.TrimPrefix(fields[len(fields)-1], "*")
			out[name] = fields[0]
		}
	}
	return out
}

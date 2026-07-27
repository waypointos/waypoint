package images

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSha256Sums(t *testing.T) {
	data := []byte("abc123  waypoint.img\n" +
		"deadbeef  waypoint-prod-0.6.0.swu\n" +
		"99  waypoint-dev-0.6.0.swu\n")
	got := ParseSha256Sums(data)
	require.Equal(t, "deadbeef", got["waypoint-prod-0.6.0.swu"])
	require.Equal(t, "99", got["waypoint-dev-0.6.0.swu"])
	require.Equal(t, "", got["missing.swu"])
}

package modules

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsNewerVersion(t *testing.T) {
	require.True(t, IsNewerVersion("0.10.0", []string{"0.9.0", "0.1.0"}))
	require.False(t, IsNewerVersion("0.9.0", []string{"0.10.0"}))
	require.False(t, IsNewerVersion("0.1.0", []string{"0.1.0"}))
	require.True(t, IsNewerVersion("1.0.0", nil))
}

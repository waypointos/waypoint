package wpmodule

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeCreds(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "creds.env")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func TestLoadCredsEnvParsesUserAndPassword(t *testing.T) {
	p := writeCreds(t, "# minted creds\nWAYPOINT_NATS_USER=mod-demo\nWAYPOINT_NATS_PASSWORD=s3cr3t\n")
	user, pass, err := loadCredsEnv(p)
	require.NoError(t, err)
	assert.Equal(t, "mod-demo", user)
	assert.Equal(t, "s3cr3t", pass)
}

func TestLoadCredsEnvRejectsMissingFields(t *testing.T) {
	p := writeCreds(t, "WAYPOINT_NATS_USER=mod-demo\n")
	_, _, err := loadCredsEnv(p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestLoadCredsEnvMissingFileErrors(t *testing.T) {
	_, _, err := loadCredsEnv(filepath.Join(t.TempDir(), "nope.env"))
	require.Error(t, err)
}

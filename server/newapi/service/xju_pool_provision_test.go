package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xju-api:new — one-click pool creation, new-api side (#4 Phase B). Exercises the
// request/poll/register flow against a fake provision dir + registry file
// (the docker work belongs to the host watcher, which isn't in scope here).

func TestSlugifyPoolID(t *testing.T) {
	cases := map[string]string{
		"Edu":           "edu",
		"K12 Edu Pool":  "k12-edu-pool",
		"  Spaces  ":    "spaces",
		"weird!!name??": "weird-name",
		"":              "",
		"---":           "",
		"UPPER":         "upper",
	}
	for in, want := range cases {
		assert.Equal(t, want, slugifyPoolID(in), "slug of %q", in)
	}
}

func TestProvisionDisabled(t *testing.T) {
	t.Setenv("POOL_PROVISION_DIR", "")
	assert.False(t, ProvisionEnabled())
	_, err := RequestPoolProvision("x")
	assert.Error(t, err)
	_, err = PollPoolProvision("x")
	assert.Error(t, err)
}

func TestPoolProvisionFlow(t *testing.T) {
	// Isolate: no env pools, fresh registry + provision dirs.
	t.Setenv("POOL_MGMT_SECRET", "")
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	provDir := t.TempDir()
	t.Setenv("POOL_PROVISION_DIR", provDir)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(t.TempDir(), "pools.json"))

	// Request → writes a create request the watcher will pick up.
	id, err := RequestPoolProvision("Edu Pool")
	require.NoError(t, err)
	assert.Equal(t, "edu-pool", id)
	reqData, err := os.ReadFile(filepath.Join(provDir, "requests", "edu-pool.json"))
	require.NoError(t, err)
	assert.Contains(t, string(reqData), `"action":"create"`)
	assert.Contains(t, string(reqData), `"pool_id":"edu-pool"`)
	assert.Contains(t, string(reqData), `"port":8319`) // first free port above k12 8318

	// Reserved label is refused.
	_, err = RequestPoolProvision("default")
	assert.Error(t, err)

	// Poll before any result → still provisioning.
	status, err := PollPoolProvision("edu-pool")
	require.NoError(t, err)
	assert.Equal(t, "provisioning", status)

	resDir := filepath.Join(provDir, "results")
	require.NoError(t, os.MkdirAll(resDir, 0o755))

	// Watcher reports failure → error, pool not registered.
	require.NoError(t, os.WriteFile(filepath.Join(resDir, "edu-pool.json"),
		[]byte(`{"pool_id":"edu-pool","status":"error","error":"docker run failed"}`), 0o600))
	status, err = PollPoolProvision("edu-pool")
	assert.Equal(t, "error", status)
	assert.Error(t, err)
	_, _, ok := common.ResolvePoolMgmt("edu-pool")
	assert.False(t, ok)

	// Watcher reports success → pool registered, status ready.
	require.NoError(t, os.WriteFile(filepath.Join(resDir, "edu-pool.json"),
		[]byte(`{"pool_id":"edu-pool","label":"Edu Pool","status":"ok",`+
			`"mgmt_url":"http://cli-proxy-api-edu-pool:8319","mgmt_secret":"sec","port":8319,"internal_key":"k"}`), 0o600))
	status, err = PollPoolProvision("edu-pool")
	require.NoError(t, err)
	assert.Equal(t, "ready", status)

	base, secret, ok := common.ResolvePoolMgmt("edu-pool")
	require.True(t, ok)
	assert.Equal(t, "http://cli-proxy-api-edu-pool:8319", base)
	assert.Equal(t, "sec", secret)

	// Idempotent: a second poll stays ready without erroring on re-add.
	status, err = PollPoolProvision("edu-pool")
	require.NoError(t, err)
	assert.Equal(t, "ready", status)
}

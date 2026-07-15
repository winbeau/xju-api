package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePoolMgmt(t *testing.T) {
	t.Setenv("POOL_MGMT_URL", "http://cli-proxy-api:8317")
	t.Setenv("POOL_MGMT_SECRET", "def-secret")
	t.Setenv("POOL_K12_MGMT_URL", "http://cli-proxy-api-k12:8318")
	t.Setenv("POOL_K12_MGMT_SECRET", "k12-secret")

	base, secret, ok := ResolvePoolMgmt("")
	require.True(t, ok)
	assert.Equal(t, "http://cli-proxy-api:8317", base)
	assert.Equal(t, "def-secret", secret)

	base, secret, ok = ResolvePoolMgmt("default")
	require.True(t, ok)
	assert.Equal(t, "def-secret", secret)

	base, secret, ok = ResolvePoolMgmt("k12")
	require.True(t, ok)
	assert.Equal(t, "http://cli-proxy-api-k12:8318", base)
	assert.Equal(t, "k12-secret", secret)

	_, _, ok = ResolvePoolMgmt("nope")
	assert.False(t, ok, "unknown pool must be not-ok")
}

func TestResolvePoolMgmtUnconfiguredSecret(t *testing.T) {
	t.Setenv("POOL_MGMT_SECRET", "")
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	_, _, ok := ResolvePoolMgmt("default")
	assert.False(t, ok, "empty secret means pool is off")
	_, _, ok = ResolvePoolMgmt("k12")
	assert.False(t, ok)
}

func TestListConfiguredPools(t *testing.T) {
	t.Setenv("POOL_MGMT_SECRET", "def-secret")
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	pools := ListConfiguredPools()
	require.Len(t, pools, 1)
	assert.Equal(t, "default", pools[0].ID)

	t.Setenv("POOL_K12_MGMT_SECRET", "k12-secret")
	pools = ListConfiguredPools()
	require.Len(t, pools, 2)
	assert.Equal(t, "k12", pools[1].ID)
}

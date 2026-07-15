package controller

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// xju-api:new — cross-pool import guard (号池验活 后续:防复发). Reproduces the
// incident where the k12 zip was imported into the default pool: a file named
// "...-k12-...json" must be refused by any pool other than k12.

func TestForeignPoolMarker(t *testing.T) {
	// Configure both pools so ListConfiguredPools() sees default + k12.
	t.Setenv("POOL_MGMT_URL", "http://cli-proxy-api:8317")
	t.Setenv("POOL_MGMT_SECRET", "s")
	t.Setenv("POOL_K12_MGMT_URL", "http://cli-proxy-api-k12:8318")
	t.Setenv("POOL_K12_MGMT_SECRET", "s")

	cases := []struct {
		name       string
		file       string
		targetPool string
		wantPool   string
		wantFlag   bool
	}{
		{"k12 file into default → blocked", "alice@x.com-k12-fc4f8db5.json", "", "k12", true},
		{"k12 file into default (explicit) → blocked", "alice@x.com-k12-fc4f8db5.json", "default", "k12", true},
		{"k12 file into k12 → allowed", "alice@x.com-k12-fc4f8db5.json", "k12", "", false},
		{"plain codex file into default → allowed", "codex-kaylahill-new.json", "default", "", false},
		{"plain codex file into k12 → allowed", "codex-kaylahill-new.json", "k12", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fp, flag := foreignPoolMarker(tc.file, tc.targetPool)
			assert.Equal(t, tc.wantFlag, flag)
			assert.Equal(t, tc.wantPool, fp)
		})
	}
}

func TestForeignPoolMarker_OnlyDefaultConfigured(t *testing.T) {
	// With no k12 pool configured, the k12 marker guard is inert (nothing to
	// compare against) — a lone-default deployment keeps working unchanged.
	t.Setenv("POOL_MGMT_URL", "http://cli-proxy-api:8317")
	t.Setenv("POOL_MGMT_SECRET", "s")
	os.Unsetenv("POOL_K12_MGMT_SECRET")
	fp, flag := foreignPoolMarker("alice@x.com-k12-fc4f8db5.json", "default")
	assert.False(t, flag)
	assert.Empty(t, fp)
}

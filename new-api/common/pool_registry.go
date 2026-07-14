package common

import (
	"os"
	"strings"
)

// Account-pool management registry.
//
// xju-api runs two isolated CLIProxyAPI pools: the primary ("default") and the
// K12 pool ("k12"). Each is addressed by its own management base URL + Bearer
// secret, sourced from this process's environment. Resolving an unknown pool, or
// a pool whose secret is unset, returns ok=false so callers degrade to 503 and
// the frontend hides that pool — a deployment that wires only the default pool
// keeps working unchanged.

// PoolInfo is the id/label pair the frontend uses to render a pool selector.
type PoolInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// ResolvePoolMgmt returns the management base URL and secret for a pool id.
// "" and "default" resolve to the primary pool; "k12" to the K12 pool. ok is
// false when the pool id is unknown or its secret is not configured.
func ResolvePoolMgmt(poolID string) (baseURL string, secret string, ok bool) {
	switch strings.TrimSpace(poolID) {
	case "", "default":
		baseURL = GetEnvOrDefaultString("POOL_MGMT_URL", "http://cli-proxy-api:8317")
		secret = strings.TrimSpace(os.Getenv("POOL_MGMT_SECRET"))
	case "k12":
		baseURL = GetEnvOrDefaultString("POOL_K12_MGMT_URL", "http://cli-proxy-api-k12:8318")
		secret = strings.TrimSpace(os.Getenv("POOL_K12_MGMT_SECRET"))
	default:
		return "", "", false
	}
	if secret == "" {
		return "", "", false
	}
	return strings.TrimRight(baseURL, "/"), secret, true
}

// ListConfiguredPools returns the pools whose secret is configured, in a stable
// order (default first, then k12), for the frontend pool selector.
func ListConfiguredPools() []PoolInfo {
	pools := make([]PoolInfo, 0, 2)
	if _, _, ok := ResolvePoolMgmt("default"); ok {
		pools = append(pools, PoolInfo{ID: "default", Label: "Default"})
	}
	if _, _, ok := ResolvePoolMgmt("k12"); ok {
		pools = append(pools, PoolInfo{ID: "k12", Label: "K12"})
	}
	return pools
}

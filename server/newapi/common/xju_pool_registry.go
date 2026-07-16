package common

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// xju-api:new — account-pool management registry.
//
// xju-api addresses each CLIProxyAPI pool by its own management base URL +
// Bearer secret. Two pools are ENV-seeded and permanent: the primary
// ("default") and the K12 pool ("k12"). Any additional pools are DYNAMIC,
// created at runtime (号池验活 Part A / #4 一键开池) and persisted in a JSON
// registry file (POOL_REGISTRY_FILE) on the data volume. Resolving an unknown
// pool, or one whose secret is unset, returns ok=false so callers degrade to
// 503 and the frontend hides that pool — a deployment that wires only the
// default pool keeps working unchanged, and one with no registry file behaves
// exactly as before.

// PoolInfo is the id/label pair the frontend uses to render a pool selector.
type PoolInfo struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	BuildMode string `json:"build_mode,omitempty"`
}

// PoolEntry is a dynamically-provisioned pool's full record in the registry
// file. The env-seeded default/k12 pools are never stored here.
type PoolEntry struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	MgmtURL    string `json:"mgmt_url"`
	MgmtSecret string `json:"mgmt_secret"`
	Port       int    `json:"port,omitempty"`
	ChannelID  int    `json:"channel_id,omitempty"`
	BuildMode  string `json:"build_mode,omitempty"` // "cliproxy"(默认) | "gopool";仅 UI 引导,无服务端强制
}

// reservedPoolIDs are env-seeded and can never be created/removed dynamically.
var reservedPoolIDs = map[string]bool{"": true, "default": true, "k12": true}

// IsReservedPoolID reports whether a pool id belongs to the env-seeded pools.
func IsReservedPoolID(id string) bool {
	return reservedPoolIDs[strings.TrimSpace(id)]
}

var (
	poolRegMu      sync.RWMutex
	poolRegEntries []PoolEntry
	poolRegMtime   time.Time
	poolRegLoaded  bool
)

func poolRegistryFile() string {
	return strings.TrimSpace(os.Getenv("POOL_REGISTRY_FILE"))
}

// loadPoolRegistry returns the dynamic pool entries, caching by file mtime so a
// concurrent process (or a later write) is picked up without a restart. An
// unset path or a missing/empty file yields no dynamic pools.
func loadPoolRegistry() []PoolEntry {
	path := poolRegistryFile()
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil // no registry file yet → env-only pools
	}

	poolRegMu.RLock()
	if poolRegLoaded && info.ModTime().Equal(poolRegMtime) {
		entries := poolRegEntries
		poolRegMu.RUnlock()
		return entries
	}
	poolRegMu.RUnlock()

	poolRegMu.Lock()
	defer poolRegMu.Unlock()
	if poolRegLoaded && info.ModTime().Equal(poolRegMtime) {
		return poolRegEntries
	}
	data, err := os.ReadFile(path)
	if err != nil {
		SysError("pool registry read failed: " + err.Error())
		return poolRegEntries
	}
	var parsed []PoolEntry
	if len(strings.TrimSpace(string(data))) > 0 {
		if err := Unmarshal(data, &parsed); err != nil {
			SysError("pool registry parse failed: " + err.Error())
			return poolRegEntries
		}
	}
	// Reserved ids can never come from the file — the env owns them.
	kept := make([]PoolEntry, 0, len(parsed))
	for _, e := range parsed {
		if !reservedPoolIDs[strings.TrimSpace(e.ID)] {
			kept = append(kept, e)
		}
	}
	poolRegEntries = kept
	poolRegMtime = info.ModTime()
	poolRegLoaded = true
	return poolRegEntries
}

// ResolvePoolMgmt returns the management base URL and secret for a pool id.
// "" and "default" resolve to the primary pool and "k12" to the K12 pool, both
// from the environment; any other id is looked up in the dynamic registry. ok
// is false when the pool id is unknown or its secret is not configured.
func ResolvePoolMgmt(poolID string) (baseURL string, secret string, ok bool) {
	id := strings.TrimSpace(poolID)
	switch id {
	case "", "default":
		baseURL = GetEnvOrDefaultString("POOL_MGMT_URL", "http://cli-proxy-api:8317")
		secret = strings.TrimSpace(os.Getenv("POOL_MGMT_SECRET"))
	case "k12":
		baseURL = GetEnvOrDefaultString("POOL_K12_MGMT_URL", "http://cli-proxy-api-k12:8318")
		secret = strings.TrimSpace(os.Getenv("POOL_K12_MGMT_SECRET"))
	default:
		for _, e := range loadPoolRegistry() {
			if e.ID != id {
				continue
			}
			url := strings.TrimSpace(e.MgmtURL)
			sec := strings.TrimSpace(e.MgmtSecret)
			if url == "" || sec == "" {
				return "", "", false
			}
			return strings.TrimRight(url, "/"), sec, true
		}
		return "", "", false
	}
	if secret == "" {
		return "", "", false
	}
	return strings.TrimRight(baseURL, "/"), secret, true
}

// ListConfiguredPools returns every pool whose secret is configured, in a stable
// order (default, k12, then dynamic pools in registry order), for the frontend
// pool selector.
func ListConfiguredPools() []PoolInfo {
	pools := make([]PoolInfo, 0, 4)
	if _, _, ok := ResolvePoolMgmt("default"); ok {
		pools = append(pools, PoolInfo{ID: "default", Label: "Default", BuildMode: "cliproxy"})
	}
	if _, _, ok := ResolvePoolMgmt("k12"); ok {
		pools = append(pools, PoolInfo{ID: "k12", Label: "K12", BuildMode: "cliproxy"})
	}
	for _, e := range loadPoolRegistry() {
		if strings.TrimSpace(e.MgmtSecret) == "" {
			continue
		}
		label := strings.TrimSpace(e.Label)
		if label == "" {
			label = e.ID
		}
		bm := strings.TrimSpace(e.BuildMode)
		if bm == "" {
			bm = "cliproxy"
		}
		pools = append(pools, PoolInfo{ID: e.ID, Label: label, BuildMode: bm})
	}
	return pools
}

// ---------------------------------------------------------------------------
// Registry mutation (used by #4 一键开池 provisioning, Phase B/C). Writes are
// atomic (temp + rename) and invalidate the mtime cache so the next read
// reflects the change.

// SavePoolRegistry persists the full dynamic pool list.
func SavePoolRegistry(entries []PoolEntry) error {
	path := poolRegistryFile()
	if path == "" {
		return fmt.Errorf("POOL_REGISTRY_FILE is not configured")
	}
	data, err := Marshal(entries)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	poolRegMu.Lock()
	poolRegLoaded = false
	poolRegMu.Unlock()
	return nil
}

// AddPoolToRegistry appends a new dynamic pool. It rejects reserved ids and
// duplicates.
func AddPoolToRegistry(entry PoolEntry) error {
	id := strings.TrimSpace(entry.ID)
	if id == "" || reservedPoolIDs[id] {
		return fmt.Errorf("invalid or reserved pool id: %q", id)
	}
	entries := append([]PoolEntry(nil), loadPoolRegistry()...)
	for _, e := range entries {
		if e.ID == id {
			return fmt.Errorf("pool already exists: %s", id)
		}
	}
	entry.ID = id
	entries = append(entries, entry)
	return SavePoolRegistry(entries)
}

// GetPoolEntry returns a dynamic pool's full record (incl. channel id) by id.
func GetPoolEntry(id string) (PoolEntry, bool) {
	id = strings.TrimSpace(id)
	for _, e := range loadPoolRegistry() {
		if e.ID == id {
			return e, true
		}
	}
	return PoolEntry{}, false
}

// SetPoolChannelID records the new-api channel that routes a dynamic pool, so a
// later delete can remove it. No-op-safe: errors if the pool is unknown.
func SetPoolChannelID(id string, channelID int) error {
	id = strings.TrimSpace(id)
	entries := append([]PoolEntry(nil), loadPoolRegistry()...)
	for i := range entries {
		if entries[i].ID == id {
			entries[i].ChannelID = channelID
			return SavePoolRegistry(entries)
		}
	}
	return fmt.Errorf("pool not found: %s", id)
}

// RemovePoolFromRegistry drops a dynamic pool by id.
func RemovePoolFromRegistry(id string) error {
	id = strings.TrimSpace(id)
	entries := loadPoolRegistry()
	out := make([]PoolEntry, 0, len(entries))
	found := false
	for _, e := range entries {
		if e.ID == id {
			found = true
			continue
		}
		out = append(out, e)
	}
	if !found {
		return fmt.Errorf("pool not found: %s", id)
	}
	return SavePoolRegistry(out)
}

// AllocateNextPoolPort returns the next free management port above the highest
// currently in use — env default 8317 / k12 8318, plus any dynamic entries.
func AllocateNextPoolPort() int {
	highest := 8318 // k12
	for _, e := range loadPoolRegistry() {
		if e.Port > highest {
			highest = e.Port
		}
	}
	return highest + 1
}

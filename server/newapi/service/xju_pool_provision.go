package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// xju-api:new — one-click pool creation (#4 Phase B, B2 host-helper side).
//
// new-api never touches the docker socket. To create an isolated cliproxy pool
// it drops a request file the host watcher (deploy/provision-poold.sh) picks up;
// the watcher provisions the container and writes a result file that carries the
// new pool's management URL/secret + relay key. new-api polls that result and,
// on success, registers the pool (common.AddPoolToRegistry). The contract:
//
//   POOL_PROVISION_DIR/requests/<id>.json   ← new-api writes {action,pool_id,label,port}
//   POOL_PROVISION_DIR/results/<id>.json     → watcher writes {status,mgmt_url,mgmt_secret,...}
//
// POOL_PROVISION_DIR unset → the feature is off and every call errors cleanly.

func provisionDir() string { return strings.TrimSpace(os.Getenv("POOL_PROVISION_DIR")) }

// ProvisionEnabled reports whether one-click pool creation is wired up.
func ProvisionEnabled() bool { return provisionDir() != "" }

// pendingMode remembers the build mode chosen at RequestPoolProvision time so
// PollPoolProvision can stamp it onto the registry entry. The host watcher
// provisions an identical container regardless of mode, so mode never round-trips
// through the result file. Kept in-memory: a new-api restart mid-provision loses
// it and the pool registers as "cliproxy" (benign — BuildMode is a UI label only).
var (
	pendingModeMu sync.Mutex
	pendingMode   = map[string]string{}
)

// normalizeBuildMode maps any input to the two supported modes, defaulting
// unknown/empty values to "cliproxy".
func normalizeBuildMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "gopool") {
		return "gopool"
	}
	return "cliproxy"
}

type provisionResult struct {
	PoolID      string `json:"pool_id"`
	Label       string `json:"label"`
	Action      string `json:"action"`
	Status      string `json:"status"`
	MgmtURL     string `json:"mgmt_url"`
	MgmtSecret  string `json:"mgmt_secret"`
	Port        int    `json:"port"`
	InternalKey string `json:"internal_key"`
	Error       string `json:"error"`
}

// slugifyPoolID turns a human label into a safe pool id: lowercase, alnum runs
// joined by single dashes, trimmed, capped at 31 chars. Matches the watcher's
// valid_pool_id guard.
func slugifyPoolID(label string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(label)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case b.Len() > 0 && !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 31 {
		slug = strings.Trim(slug[:31], "-")
	}
	return slug
}

// RequestPoolProvision validates the label, allocates a port, and drops a create
// request for the host watcher. Returns the derived pool id.
func RequestPoolProvision(label, mode string) (string, error) {
	dir := provisionDir()
	if dir == "" {
		return "", fmt.Errorf("pool provisioning is not enabled")
	}
	id := slugifyPoolID(label)
	if id == "" {
		return "", fmt.Errorf("invalid pool name")
	}
	if common.IsReservedPoolID(id) {
		return "", fmt.Errorf("reserved pool id: %s", id)
	}
	if _, _, ok := common.ResolvePoolMgmt(id); ok {
		return "", fmt.Errorf("pool already exists: %s", id)
	}
	m := normalizeBuildMode(mode)
	pendingModeMu.Lock()
	pendingMode[id] = m
	pendingModeMu.Unlock()
	req := map[string]any{
		"action":  "create",
		"pool_id": id,
		"label":   strings.TrimSpace(label),
		"port":    common.AllocateNextPoolPort(),
		"mode":    m,
	}
	if err := writeProvisionRequest(dir, id, req); err != nil {
		return "", err
	}
	return id, nil
}

// PollPoolProvision reports a create's status: "ready" once the pool is
// registered, "provisioning" while the watcher works, or an error. It is
// idempotent — the first poll that sees a successful result registers the pool.
func PollPoolProvision(poolID string) (string, error) {
	dir := provisionDir()
	if dir == "" {
		return "", fmt.Errorf("pool provisioning is not enabled")
	}
	poolID = strings.TrimSpace(poolID)
	if _, _, ok := common.ResolvePoolMgmt(poolID); ok {
		return "ready", nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "results", poolID+".json"))
	if err != nil {
		return "provisioning", nil // no result yet
	}
	var r provisionResult
	if err := common.Unmarshal(data, &r); err != nil {
		return "", fmt.Errorf("unreadable provisioning result: %w", err)
	}
	if r.Status != "ok" {
		msg := r.Error
		if msg == "" {
			msg = "provisioning failed"
		}
		return "error", fmt.Errorf("%s", msg)
	}
	label := strings.TrimSpace(r.Label)
	if label == "" {
		label = poolID
	}
	pendingModeMu.Lock()
	mode := pendingMode[poolID]
	pendingModeMu.Unlock()
	if mode == "" {
		mode = "cliproxy"
	}
	if err := common.AddPoolToRegistry(common.PoolEntry{
		ID:         r.PoolID,
		Label:      label,
		MgmtURL:    r.MgmtURL,
		MgmtSecret: r.MgmtSecret,
		Port:       r.Port,
		BuildMode:  mode,
	}); err != nil {
		// A concurrent poll may have registered it first — treat as ready.
		if _, _, ok := common.ResolvePoolMgmt(poolID); ok {
			return "ready", nil
		}
		return "", err
	}
	pendingModeMu.Lock()
	delete(pendingMode, poolID)
	pendingModeMu.Unlock()
	// Phase C: route the pool's group to its cliproxy instance. The mgmt URL and
	// the relay base URL are the same host:port. A channel failure leaves the
	// pool registered (importable/verifiable) but unrouted — log, don't unwind.
	if chID, err := createPoolChannel(r.PoolID, r.InternalKey, r.MgmtURL, label); err != nil {
		common.SysError("pool " + r.PoolID + " registered but channel creation failed: " + err.Error())
	} else {
		_ = common.SetPoolChannelID(r.PoolID, chID)
	}
	return "ready", nil
}

// DeletePoolInstance tears down a dynamic pool: it asks the host watcher to
// remove the container, deletes the routing channel + group options, and drops
// the pool from the registry. Built-in pools (default/k12) are refused.
func DeletePoolInstance(poolID string) error {
	dir := provisionDir()
	if dir == "" {
		return fmt.Errorf("pool provisioning is not enabled")
	}
	poolID = strings.TrimSpace(poolID)
	if common.IsReservedPoolID(poolID) {
		return fmt.Errorf("cannot delete a built-in pool: %s", poolID)
	}
	entry, ok := common.GetPoolEntry(poolID)
	if !ok {
		return fmt.Errorf("pool not found: %s", poolID)
	}
	if err := writeProvisionRequest(dir, poolID, map[string]any{"action": "delete", "pool_id": poolID}); err != nil {
		return err
	}
	deletePoolChannel(poolID, entry.ChannelID)
	return common.RemovePoolFromRegistry(poolID)
}

func writeProvisionRequest(dir, id string, req map[string]any) error {
	reqDir := filepath.Join(dir, "requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		return err
	}
	data, err := common.Marshal(req)
	if err != nil {
		return err
	}
	tmp := filepath.Join(reqDir, id+".json.tmp")
	// 0644: the request carries no secrets and the host watcher (a different
	// user) must be able to read it.
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(reqDir, id+".json"))
}

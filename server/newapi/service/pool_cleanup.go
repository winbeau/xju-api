package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/bytedance/gopkg/util/gopool"
)

// Account-pool auto-clean.
//
// When PoolAutoCleanEnabled is on, an hourly sweep asks the CLIProxyAPI
// management API for the pool's accounts and disables (not deletes) any that
// have gone `unavailable` — depleted or auth-failed — and stayed that way past
// PoolAutoCleanHours since their last activity. A disabled account keeps its
// file and can be re-enabled after a top-up.
//
// It shares POOL_MGMT_URL / POOL_MGMT_SECRET with the controller-side proxy; the
// secret lives only in this process's environment. With no secret the sweep is
// a no-op, so a deployment that doesn't wire the pool degrades cleanly.

var poolCleanupOnce sync.Once

const poolCleanupInterval = time.Hour

var poolCleanupClient = &http.Client{Timeout: 20 * time.Second}

// StartPoolAutoCleanTask launches the hourly sweep. It always starts; each tick
// checks PoolAutoCleanEnabled, so the admin toggle takes effect without a restart.
func StartPoolAutoCleanTask() {
	poolCleanupOnce.Do(func() {
		gopool.Go(func() {
			ticker := time.NewTicker(poolCleanupInterval)
			defer ticker.Stop()
			for range ticker.C {
				runPoolAutoCleanOnce()
			}
		})
	})
}

func runPoolAutoCleanOnce() {
	if !common.PoolAutoCleanEnabled {
		return
	}
	if _, _, ok := common.ResolvePoolMgmt("default"); !ok {
		return
	}
	disabled, err := SweepPoolOnce(common.PoolAutoCleanHours)
	if err != nil {
		common.SysError("pool auto-clean sweep failed: " + err.Error())
		return
	}
	if disabled > 0 {
		common.SysLog(fmt.Sprintf("pool auto-clean: disabled %d stale account(s)", disabled))
	}
}

type poolAuthEntry struct {
	Name        string `json:"name"`
	Disabled    bool   `json:"disabled"`
	Unavailable bool   `json:"unavailable"`
	UpdatedAt   string `json:"updated_at"`
	LastRefresh string `json:"last_refresh"`
}

type poolListResponse struct {
	Files []poolAuthEntry `json:"files"`
}

// SweepPoolOnce sweeps the default pool. Kept for the hourly auto-clean task.
func SweepPoolOnce(hours int) (int, error) {
	return SweepPoolOnceForPool("default", hours)
}

// SweepPoolOnceForPool disables every account in the given pool that is
// unavailable and whose last activity is older than `hours`. Returns the number
// newly disabled. Exposed so the manual "clean now" button can trigger the same
// logic against any configured pool.
func SweepPoolOnceForPool(poolID string, hours int) (int, error) {
	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		return 0, fmt.Errorf("pool management is not configured for pool: %s", poolID)
	}
	if hours <= 0 {
		hours = 24
	}
	entries, err := listPoolEntries(baseURL, secret)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	disabled := 0
	for _, e := range entries {
		if e.Disabled || !e.Unavailable {
			continue
		}
		last := parsePoolTimestamp(e.LastRefresh)
		if last.IsZero() {
			last = parsePoolTimestamp(e.UpdatedAt)
		}
		// No usable timestamp → don't touch it; better to leave a possibly-fine
		// account than to disable on missing data.
		if last.IsZero() || last.After(cutoff) {
			continue
		}
		if err := disablePoolEntry(baseURL, secret, e.Name); err != nil {
			common.SysError("pool auto-clean: failed to disable " + e.Name + ": " + err.Error())
			continue
		}
		disabled++
	}
	return disabled, nil
}

func parsePoolTimestamp(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, v); err == nil {
			return t
		}
	}
	return time.Time{}
}

func listPoolEntries(baseURL, secret string) ([]poolAuthEntry, error) {
	body, err := poolMgmtRequest(baseURL, secret, http.MethodGet, "/v0/management/auth-files", nil)
	if err != nil {
		return nil, err
	}
	var parsed poolListResponse
	if err := common.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return parsed.Files, nil
}

func disablePoolEntry(baseURL, secret, name string) error {
	payload, err := common.Marshal(map[string]any{"name": name, "disabled": true})
	if err != nil {
		return err
	}
	_, err = poolMgmtRequest(baseURL, secret, http.MethodPatch, "/v0/management/auth-files/status", strings.NewReader(string(payload)))
	return err
}

func poolMgmtRequest(baseURL, secret, method, path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), method, baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := poolCleanupClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pool management HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

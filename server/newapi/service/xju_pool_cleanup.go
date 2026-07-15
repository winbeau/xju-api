package service

import (
	"fmt"
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
	// xju-api:edit — 多实例守卫(REFACTOR-PLAN §5.2):sweep 会改池内账号状态,
	// 多节点部署时只允许 master 执行,避免重复清理与并发状态写。
	if !common.IsMasterNode {
		return
	}
	// xju-api:edit — 多池决断(owner 拍板纳入):遍历全部已配置池逐池 sweep,
	// 消灭「default 自动清、K12 只能手动清」的隐性行为差异。
	for _, pool := range common.ListConfiguredPools() {
		disabled, err := SweepPoolOnceForPool(pool.ID, common.PoolAutoCleanHours)
		if err != nil {
			common.SysError("pool auto-clean sweep failed for pool " + pool.ID + ": " + err.Error())
			continue
		}
		if disabled > 0 {
			common.SysLog(fmt.Sprintf("pool auto-clean: disabled %d stale account(s) in pool %s", disabled, pool.ID))
		}
	}
}

type poolAuthEntry struct {
	Name        string `json:"name"`
	AuthIndex   string `json:"auth_index"`
	Disabled    bool   `json:"disabled"`
	Unavailable bool   `json:"unavailable"`
	UpdatedAt   string `json:"updated_at"`
	LastRefresh string `json:"last_refresh"`
	// xju-api:new — codex accounts carry their ChatGPT subscription window here;
	// an expired subscription is a certain death the sweep can act on directly.
	// AccountID is the chatgpt-account-id header the wham quota endpoints need.
	IDToken struct {
		SubscriptionActiveUntil string `json:"chatgpt_subscription_active_until"`
		AccountID               string `json:"chatgpt_account_id"`
	} `json:"id_token"`
}

// subscriptionExpired reports whether a codex account's ChatGPT subscription
// window has already closed. Non-codex accounts (no id_token) return false.
func (e poolAuthEntry) subscriptionExpired(now time.Time) bool {
	until := parsePoolTimestamp(e.IDToken.SubscriptionActiveUntil)
	return !until.IsZero() && until.Before(now)
}

type poolListResponse struct {
	Files []poolAuthEntry `json:"files"`
}

// SweepPoolOnceForPool disables accounts in the given pool that are either
// (a) unavailable and idle past `hours`, or (b) on an expired subscription. It
// returns how many were newly disabled. Exposed so the manual "clean now"
// button can trigger the same logic against any configured pool.
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

	now := time.Now()
	cutoff := now.Add(-time.Duration(hours) * time.Hour)
	disabled := 0
	for _, e := range entries {
		if e.Disabled {
			continue
		}
		switch {
		// xju-api:new — expired subscription is a certain death: disable
		// immediately, regardless of cooldown or idle time.
		case e.subscriptionExpired(now):
		case e.Unavailable:
			last := parsePoolTimestamp(e.LastRefresh)
			if last.IsZero() {
				last = parsePoolTimestamp(e.UpdatedAt)
			}
			// No usable timestamp, or still within the idle window → leave it;
			// better a possibly-fine account than disabling on missing data.
			if last.IsZero() || last.After(cutoff) {
				continue
			}
		default:
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
	// xju-api:edit — round-trip 单一来源:改走 xju_pool_client.go 的 PoolMgmtRequest
	body, err := PoolMgmtRequest(baseURL, secret, http.MethodGet, "/v0/management/auth-files", nil)
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
	_, err = PoolMgmtRequest(baseURL, secret, http.MethodPatch, "/v0/management/auth-files/status", strings.NewReader(string(payload)))
	return err
}

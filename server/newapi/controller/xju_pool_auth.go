package controller

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// xju-api:new — pool-authentication proxy.
//
// xju-api runs a three-tier stack: this new-api instance (L1) sits in front of
// a CLIProxyAPI account pool (L2). Adding an upstream account means dropping a
// codex auth JSON into the pool, which CLIProxyAPI exposes through its
// management API (`/v0/management/auth-files`). That API is bound to the
// internal network and gated by a management secret, so the browser can neither
// reach it nor be trusted with the secret. These handlers are the thin,
// root-only bridge: the secret lives only here, in this process's environment.
//
//   POOL_MGMT_URL     base URL of the pool management API
//                     (default http://cli-proxy-api:8317 — the docker service name)
//   POOL_MGMT_SECRET  the CLIProxyAPI `remote-management.secret-key`
//
// When POOL_MGMT_SECRET is empty the feature is simply off and every handler
// answers 503, so a deployment that doesn't wire it up degrades cleanly.

// poolMgmtProxy forwards a request to the given pool's management API and copies
// the upstream status + body back to the caller under the uniform envelope.
// It reports whether the upstream call succeeded so mutating handlers can write
// their audit entry only for operations that actually happened.
// HTTP 传输走 service.PoolMgmtRoundTrip(round-trip 单一来源,REFACTOR-PLAN §5.2)。
func poolMgmtProxy(c *gin.Context, poolID, method, path string, body io.Reader, contentType string) bool {
	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "pool management is not configured for pool: " + poolID,
		})
		return false
	}
	status, payload, err := service.PoolMgmtRoundTrip(c.Request.Context(), baseURL, secret, method, path, body, contentType)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": fmt.Sprintf("pool management unreachable: %v", err),
		})
		return false
	}
	if status >= 200 && status < 300 {
		c.Data(http.StatusOK, "application/json; charset=utf-8", wrapPoolSuccess(payload))
		return true
	}
	c.JSON(status, gin.H{
		"success": false,
		"message": poolErrorMessage(payload, status),
	})
	return false
}

// ListPools GET /api/pool/pools — the configured pools (default + k12) so the
// frontend can render a pool selector and hide unconfigured pools.
func ListPools(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": common.ListConfiguredPools()})
}

func wrapPoolSuccess(raw []byte) []byte {
	// Return the upstream body verbatim under `data` without re-marshaling the
	// (already valid) JSON, so list responses keep their exact shape.
	var buf strings.Builder
	buf.WriteString(`{"success":true,"data":`)
	if len(raw) == 0 {
		buf.WriteString("null")
	} else {
		buf.Write(raw)
	}
	buf.WriteString(`}`)
	return []byte(buf.String())
}

func poolErrorMessage(raw []byte, status int) string {
	var parsed struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := common.Unmarshal(raw, &parsed); err == nil {
		if parsed.Error != "" {
			return parsed.Error
		}
		if parsed.Message != "" {
			return parsed.Message
		}
	}
	return fmt.Sprintf("pool management returned HTTP %d", status)
}

// ListPoolAuthFiles GET /api/pool/auth-files — the accounts currently in the pool.
func ListPoolAuthFiles(c *gin.Context) {
	poolMgmtProxy(c, c.Query("pool"), http.MethodGet, "/v0/management/auth-files", nil, "")
}

type addPoolAuthFileRequest struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// AddPoolAuthFile POST /api/pool/auth-files — paste one auth JSON into the pool.
// CLIProxyAPI hot-reloads the pool on write, so no container restart is needed.
func AddPoolAuthFile(c *gin.Context) {
	var reqBody addPoolAuthFileRequest
	if err := common.DecodeJson(c.Request.Body, &reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}

	content := strings.TrimSpace(reqBody.Content)
	if content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "content is empty"})
		return
	}
	// Validate it is real JSON before it ever reaches the pool — a malformed
	// paste should fail here with a clear message, not on the pool side.
	var probe any
	if err := common.UnmarshalJsonStr(content, &probe); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "content is not valid JSON"})
		return
	}

	poolID := c.Query("pool")
	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "pool management is not configured for pool: " + poolID,
		})
		return
	}

	// A pasted blob can be one codex object, or an exporter bundle / JSON array
	// holding many accounts; expand it so every account is imported, not just the
	// first. Names come from each account's email.
	items := parsePoolAuthAccounts(content, reqBody.Name)
	if len(items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "no account found in the JSON"})
		return
	}

	imported, failed, skipped, err := forwardPoolAuthItems(c.Request.Context(), baseURL, secret, poolID, items)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}

	// xju-api:edit — 审计只记池与计数,凭证内容/文件名永不入审计。
	recordManageAudit(c, "pool_auth.add", map[string]interface{}{
		"pool":     auditPoolID(poolID),
		"imported": imported,
		"skipped":  len(skipped),
		"failed":   len(failed),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"imported": imported,
			"skipped":  skipped,
			"failed":   failed,
		},
	})
}

// poolAuthItem is one single-account auth file ready to upload: a pool-safe
// basename and the exact JSON the pool stores.
type poolAuthItem struct {
	name    string
	content string
}

// parsePoolAuthAccounts expands a pasted or uploaded JSON blob into one or more
// single-account items. It accepts a bare codex auth object, an exporter bundle
// {accounts:[{credentials:{...}}]}, or a JSON array of either — so one file or one
// paste can carry many accounts and every one of them is imported, not just the
// first. Each expanded account's name is derived from its email as
// codex-<slug>.json (mirroring the frontend), so a bundle of N accounts yields N
// distinct files. rawName is the fallback name only for a single bare object with
// no email (the frontend already supplies one). A blob that is not valid JSON
// passes through unchanged as one item so the pool side returns the parse error.
func parsePoolAuthAccounts(content, rawName string) []poolAuthItem {
	trimmed := strings.TrimSpace(content)
	var top any
	if err := common.UnmarshalJsonStr(trimmed, &top); err != nil {
		return []poolAuthItem{{name: sanitizePoolAuthName(rawName), content: trimmed}}
	}
	switch v := top.(type) {
	case map[string]any:
		if accounts, ok := v["accounts"].([]any); ok && len(accounts) > 0 {
			return expandPoolAuthAccounts(accounts)
		}
		return []poolAuthItem{singlePoolAuthItem(v, rawName, trimmed)}
	case []any:
		return expandPoolAuthAccounts(v)
	default:
		return []poolAuthItem{{name: sanitizePoolAuthName(rawName), content: trimmed}}
	}
}

// expandPoolAuthAccounts turns a list of account entries — each either a bundle
// account with a nested `credentials`, or a bare codex object — into items,
// dropping any element that is not an object or cannot be re-marshaled.
func expandPoolAuthAccounts(list []any) []poolAuthItem {
	items := make([]poolAuthItem, 0, len(list))
	for i, el := range list {
		obj, ok := el.(map[string]any)
		if !ok {
			continue
		}
		cred := obj
		accountName := stringField(obj, "name")
		if inner, ok := obj["credentials"].(map[string]any); ok {
			cred = inner
		}
		raw, err := common.Marshal(cred)
		if err != nil {
			continue
		}
		items = append(items, poolAuthItem{
			name:    poolAuthAccountName(cred, accountName, i),
			content: string(raw),
		})
	}
	return items
}

// singlePoolAuthItem wraps a bare codex object, keeping the caller-supplied name
// when present (the frontend derives it) and otherwise deriving one from the
// object's email.
func singlePoolAuthItem(obj map[string]any, rawName, content string) poolAuthItem {
	name := strings.TrimSpace(rawName)
	if name == "" {
		name = poolAuthAccountName(obj, "", 0)
	}
	return poolAuthItem{name: sanitizePoolAuthName(name), content: content}
}

// poolAuthAccountName builds codex-<slug>.json from the account email, falling
// back to the account's display name, then a 1-based index, so every expanded
// account gets a stable, collision-free basename.
func poolAuthAccountName(cred map[string]any, accountName string, index int) string {
	if slug := poolAuthSlug(stringField(cred, "email")); slug != "" {
		return "codex-" + slug + ".json"
	}
	if slug := poolAuthSlug(accountName); slug != "" {
		return "codex-" + slug + ".json"
	}
	return fmt.Sprintf("codex-account-%d.json", index+1)
}

// poolAuthSlug lowercases a string and collapses non-alphanumeric runs into single
// dashes (capped at 48 chars), matching the frontend deriveAuthFileName so the
// same account resolves to the same file whether added singly or via a bundle.
func poolAuthSlug(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
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
	if len(slug) > 48 {
		slug = strings.Trim(slug[:48], "-")
	}
	return slug
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

type patchPoolAuthStatusRequest struct {
	Name     string `json:"name"`
	Disabled *bool  `json:"disabled"`
}

// SetPoolAuthFileStatus PATCH /api/pool/auth-files/status — disable/enable one
// account without deleting it (a depleted account can be re-enabled after top-up).
func SetPoolAuthFileStatus(c *gin.Context) {
	var reqBody patchPoolAuthStatusRequest
	if err := common.DecodeJson(c.Request.Body, &reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	if strings.TrimSpace(reqBody.Name) == "" || reqBody.Disabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "name and disabled are required"})
		return
	}
	name := sanitizePoolAuthName(reqBody.Name)
	body, err := common.Marshal(map[string]any{
		"name":     name,
		"disabled": *reqBody.Disabled,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ok := poolMgmtProxy(
		c, c.Query("pool"), http.MethodPatch, "/v0/management/auth-files/status",
		strings.NewReader(string(body)), "application/json",
	)
	if ok {
		recordManageAudit(c, "pool_auth.status", map[string]interface{}{
			"pool":     auditPoolID(c.Query("pool")),
			"name":     name,
			"disabled": *reqBody.Disabled,
		})
	}
}

// CleanPoolAuthFilesNow POST /api/pool/auth-files/clean — run the stale-account
// sweep on demand (same logic as the hourly auto-clean), using the current
// PoolAutoCleanHours threshold. Returns how many accounts were disabled.
func CleanPoolAuthFilesNow(c *gin.Context) {
	poolID := c.Query("pool")
	if _, _, ok := common.ResolvePoolMgmt(poolID); !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "pool management is not configured for pool: " + poolID,
		})
		return
	}
	disabled, err := service.SweepPoolOnceForPool(poolID, common.PoolAutoCleanHours)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "pool_auth.clean", map[string]interface{}{
		"pool":     auditPoolID(poolID),
		"disabled": disabled,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"disabled": disabled}})
}

// DeletePoolAuthFile DELETE /api/pool/auth-files?name=xxx — remove one account.
func DeletePoolAuthFile(c *gin.Context) {
	name := sanitizePoolAuthName(c.Query("name"))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "name is required"})
		return
	}
	ok := poolMgmtProxy(
		c, c.Query("pool"), http.MethodDelete,
		"/v0/management/auth-files?name="+url.QueryEscape(name),
		nil, "",
	)
	if ok {
		recordManageAudit(c, "pool_auth.delete", map[string]interface{}{
			"pool": auditPoolID(c.Query("pool")),
			"name": name,
		})
	}
}

// xju-api:new — active verification (号池验活 Part A). These handlers verify
// whether pool accounts are actually online by pinning a probe request to each
// via cliproxy's api-call, rather than trusting the passively-set `unavailable`
// flag. Single verify only reports; verify-all can opt into auto-disabling the
// accounts it finds genuinely dead.

type verifyPoolAuthFileRequest struct {
	Name  string `json:"name"`
	Heavy bool   `json:"heavy"`
}

// VerifyPoolAuthFile POST /api/pool/auth-files/verify — verify one account and
// return its verdict. Report-only: it never changes account state.
func VerifyPoolAuthFile(c *gin.Context) {
	var reqBody verifyPoolAuthFileRequest
	if err := common.DecodeJson(c.Request.Body, &reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	name := sanitizePoolAuthName(reqBody.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "name is required"})
		return
	}
	result, err := service.ProbeAuthByName(c.Query("pool"), name, reqBody.Heavy)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "pool_auth.verify", map[string]interface{}{
		"pool":    auditPoolID(c.Query("pool")),
		"name":    name,
		"verdict": string(result.Verdict),
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

type verifyPoolAllRequest struct {
	Heavy       bool `json:"heavy"`
	AutoDisable bool `json:"auto_disable"`
}

// VerifyPoolAuthFilesNow POST /api/pool/auth-files/verify-all — start a
// background full-pool verification and return the initial job snapshot. The
// frontend polls the progress endpoint. Refuses to start a second run while one
// is already in flight for the pool.
func VerifyPoolAuthFilesNow(c *gin.Context) {
	var reqBody verifyPoolAllRequest
	// Body is optional (both flags default false); ignore decode errors on empty.
	_ = common.DecodeJson(c.Request.Body, &reqBody)

	snap, err := service.StartProbePoolJob(c.Query("pool"), reqBody.Heavy, reqBody.AutoDisable)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error(), "data": snap})
		return
	}
	recordManageAudit(c, "pool_auth.verify_all", map[string]interface{}{
		"pool":         auditPoolID(c.Query("pool")),
		"heavy":        reqBody.Heavy,
		"auto_disable": reqBody.AutoDisable,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": snap})
}

// GetVerifyPoolProgress GET /api/pool/auth-files/verify-all/progress — the
// latest verify-all job snapshot for the pool (running or finished).
func GetVerifyPoolProgress(c *gin.Context) {
	snap, ok := service.GetProbePoolJob(c.Query("pool"))
	if !ok {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": snap})
}

// xju-api:new — per-account quota (额度). GET returns the cached snapshots +
// refresh-job progress; refresh updates one account synchronously or the whole
// pool in the background; reset consumes one ChatGPT reset credit on demand.

// GetPoolAccountUsage GET /api/pool/auth-files/usage — cached quota snapshots
// (keyed by account file name) plus the latest refresh-job progress.
func GetPoolAccountUsage(c *gin.Context) {
	poolID := c.Query("pool")
	if _, _, ok := common.ResolvePoolMgmt(poolID); !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "pool management is not configured for pool: " + poolID,
		})
		return
	}
	data := gin.H{"accounts": service.GetPoolUsageSnapshots(poolID)}
	if snap, ok := service.GetPoolUsageJob(poolID); ok {
		data["job"] = snap
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

type refreshPoolUsageRequest struct {
	Name string `json:"name"`
}

// RefreshPoolAccountUsage POST /api/pool/auth-files/usage/refresh — with a name,
// refresh that account's quota synchronously and return it; without one, start
// a background whole-pool refresh (poll GetPoolAccountUsage for progress).
func RefreshPoolAccountUsage(c *gin.Context) {
	var reqBody refreshPoolUsageRequest
	// Body is optional (empty means whole-pool); ignore decode errors on empty.
	_ = common.DecodeJson(c.Request.Body, &reqBody)

	poolID := c.Query("pool")
	name := strings.TrimSpace(reqBody.Name)
	if name != "" {
		usage, err := service.RefreshPoolAccountUsageByName(poolID, sanitizePoolAuthName(name))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": usage})
		return
	}

	// The manual whole-pool button is targeted: only exhausted/unknown accounts
	// are re-fetched; accounts with quota left are skipped.
	snap, err := service.StartPoolUsageRefreshJob(poolID, common.PoolUsageAutoResetEnabled, true)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error(), "data": snap})
		return
	}
	recordManageAudit(c, "pool_auth.usage_refresh", map[string]interface{}{
		"pool": auditPoolID(poolID),
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": snap})
}

type resetPoolQuotaRequest struct {
	Name string `json:"name"`
}

// ResetPoolAccountQuota POST /api/pool/auth-files/usage/reset — consume one
// ChatGPT reset credit on the account and return its renewed quota snapshot.
func ResetPoolAccountQuota(c *gin.Context) {
	var reqBody resetPoolQuotaRequest
	if err := common.DecodeJson(c.Request.Body, &reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	name := sanitizePoolAuthName(reqBody.Name)
	if strings.TrimSpace(reqBody.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "name is required"})
		return
	}
	usage, err := service.ResetPoolAccountQuota(c.Query("pool"), name)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "pool_auth.quota_reset", map[string]interface{}{
		"pool": auditPoolID(c.Query("pool")),
		"name": name,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": usage})
}

// xju-api:new — one-click pool creation (#4 Phase B). CreatePoolInstance drops a
// provisioning request for the host watcher; the frontend then polls
// GetPoolCreateStatus until the new pool is registered. Both are root-only.

type createPoolRequest struct {
	Label string `json:"label"`
	Mode  string `json:"mode"`
}

// CreatePoolInstance POST /api/pool/create — start provisioning a new isolated
// pool from a display label. Returns the derived pool id; the actual container
// is brought up asynchronously by the host watcher.
func CreatePoolInstance(c *gin.Context) {
	if !service.ProvisionEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "pool provisioning is not enabled on this deployment"})
		return
	}
	var reqBody createPoolRequest
	if err := common.DecodeJson(c.Request.Body, &reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	poolID, err := service.RequestPoolProvision(reqBody.Label, reqBody.Mode)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "pool_auth.create", map[string]interface{}{
		"pool":  poolID,
		"label": strings.TrimSpace(reqBody.Label),
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"pool_id": poolID, "status": "provisioning"}})
}

type deletePoolInstanceRequest struct {
	PoolID string `json:"pool_id"`
}

// DeletePoolInstance POST /api/pool/delete — tear down a dynamically-created
// pool (container + routing channel + registry). Built-in pools are refused.
func DeletePoolInstance(c *gin.Context) {
	if !service.ProvisionEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "pool provisioning is not enabled on this deployment"})
		return
	}
	var reqBody deletePoolInstanceRequest
	if err := common.DecodeJson(c.Request.Body, &reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	poolID := strings.TrimSpace(reqBody.PoolID)
	if err := service.DeletePoolInstance(poolID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "pool_auth.delete_pool", map[string]interface{}{"pool": poolID})
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetPoolCreateStatus GET /api/pool/create/status?id=xxx — poll provisioning
// progress. On success the pool is registered and status becomes "ready".
func GetPoolCreateStatus(c *gin.Context) {
	poolID := strings.TrimSpace(c.Query("id"))
	if poolID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "id is required"})
		return
	}
	status, err := service.PollPoolProvision(poolID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"pool_id": poolID, "status": "error", "error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"pool_id": poolID, "status": status}})
}

// auditPoolID normalizes the pool query param for audit entries ("" resolves
// to the default pool, mirroring common.ResolvePoolMgmt).
func auditPoolID(poolID string) string {
	if strings.TrimSpace(poolID) == "" {
		return "default"
	}
	return poolID
}

// xju-api:new — cross-pool import guard. The batch importer stamps a pool's
// files with "-<poolID>-" in their name (e.g. "alice@x.com-k12-<hash>.json"), so
// a file whose name carries a *different* configured pool's marker almost
// certainly belongs to that other pool. This catches the real incident where an
// operator imported the k12 zip into the default pool by forgetting to switch
// the pool selector, silently polluting it with 500 accounts. Returns the
// foreign pool id when the name looks misrouted.
func foreignPoolMarker(name, targetPool string) (string, bool) {
	target := auditPoolID(targetPool)
	lower := strings.ToLower(name)
	for _, p := range common.ListConfiguredPools() {
		if p.ID == target {
			continue
		}
		if strings.Contains(lower, "-"+strings.ToLower(p.ID)+"-") {
			return p.ID, true
		}
	}
	return "", false
}

// sanitizePoolAuthName keeps the name a plain `*.json` basename. The pool side
// has its own guard, but stripping path separators here means a bad name can
// never be composed into a traversal against the management API.
func sanitizePoolAuthName(raw string) string {
	name := strings.TrimSpace(raw)
	name = strings.ReplaceAll(name, "\\", "/")
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	if name == "" {
		name = fmt.Sprintf("codex-%d.json", time.Now().Unix())
	}
	if !strings.HasSuffix(strings.ToLower(name), ".json") {
		name += ".json"
	}
	return name
}

// Batch-import limits: keep a hostile upload from exhausting memory. The real
// K12 zip is <1MB / 500 files, so these ceilings are generous.
const (
	maxImportZipBytes   = 64 << 20 // 64 MiB uploaded zip
	maxImportEntries    = 2000     // process at most this many entries
	maxImportEntryBytes = 1 << 20  // 1 MiB per JSON entry
)

type importSkip struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type importFail struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

// forwardPoolAuthItems uploads one or more single-account items to the pool's
// bulk auth-files endpoint in a single multipart request and returns the merged
// {imported, failed} plus any items skipped locally (foreign-pool markers). It is
// shared by AddPoolAuthFile (paste / single upload) and ImportPoolAuthFiles (zip),
// so all three entry points import multi-account blobs identically.
func forwardPoolAuthItems(ctx context.Context, baseURL, secret, poolID string, items []poolAuthItem) (imported int, failed []importFail, skipped []importSkip, err error) {
	failed = make([]importFail, 0)
	skipped = make([]importSkip, 0)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	forwarded := 0
	for _, it := range items {
		if fp, misrouted := foreignPoolMarker(it.name, poolID); misrouted {
			skipped = append(skipped, importSkip{Name: it.name, Reason: "belongs to pool " + fp + " (name contains -" + fp + "-)"})
			continue
		}
		part, cErr := mw.CreateFormFile("files", it.name)
		if cErr != nil {
			skipped = append(skipped, importSkip{Name: it.name, Reason: "internal error"})
			continue
		}
		if _, wErr := part.Write([]byte(it.content)); wErr != nil {
			skipped = append(skipped, importSkip{Name: it.name, Reason: "internal error"})
			continue
		}
		forwarded++
	}
	if cErr := mw.Close(); cErr != nil {
		return 0, failed, skipped, cErr
	}
	if forwarded == 0 {
		return 0, failed, skipped, nil
	}
	status, payload, rErr := service.PoolMgmtRoundTrip(
		ctx, baseURL, secret,
		http.MethodPost, "/v0/management/auth-files", &body, mw.FormDataContentType(),
	)
	if rErr != nil {
		return 0, failed, skipped, fmt.Errorf("pool management unreachable: %v", rErr)
	}
	if status < 200 || status >= 300 {
		return 0, failed, skipped, fmt.Errorf("%s", poolErrorMessage(payload, status))
	}
	var parsed struct {
		Uploaded int          `json:"uploaded"`
		Files    []string     `json:"files"`
		Failed   []importFail `json:"failed"`
	}
	if uErr := common.Unmarshal(payload, &parsed); uErr == nil {
		failed = append(failed, parsed.Failed...)
		imported = parsed.Uploaded
		if imported == 0 && len(parsed.Files) > 0 {
			imported = len(parsed.Files)
		}
		if imported == 0 && len(parsed.Failed) == 0 {
			imported = forwarded
		}
	} else {
		imported = forwarded
	}
	return imported, failed, skipped, nil
}

// ImportPoolAuthFiles POST /api/pool/auth-files/import?pool=xxx — accept a .zip
// of codex auth JSON files, expand it server-side, and forward every valid entry
// as one multipart batch to the target pool's management API. Locally-skipped
// entries (non-json, malformed, oversize) are merged with the pool's per-file
// failures into a single {imported, skipped, failed} report. No file is written
// to disk here and only the entry's base name is used, so there is no zip-slip
// surface; token contents are never logged.
func ImportPoolAuthFiles(c *gin.Context) {
	poolID := c.Query("pool")
	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "pool management is not configured for pool: " + poolID,
		})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "no zip uploaded (field 'file')"})
		return
	}
	if fileHeader.Size > maxImportZipBytes {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "zip too large"})
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "cannot read upload"})
		return
	}
	defer f.Close()
	zipBytes, err := io.ReadAll(io.LimitReader(f, maxImportZipBytes))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "cannot read upload"})
		return
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid zip"})
		return
	}

	items := make([]poolAuthItem, 0)
	skipped := make([]importSkip, 0)
	seen := 0

	for _, entry := range zr.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		base := poolZipEntryBase(entry.Name)
		if strings.HasPrefix(base, ".") || strings.Contains(entry.Name, "__MACOSX/") {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(base), ".json") {
			skipped = append(skipped, importSkip{Name: base, Reason: "not a .json file"})
			continue
		}
		seen++
		if seen > maxImportEntries {
			skipped = append(skipped, importSkip{Name: base, Reason: "entry limit reached, skipped"})
			common.SysLog("pool import: entry limit " + strconv.Itoa(maxImportEntries) + " reached, extra entries skipped")
			continue
		}
		if entry.UncompressedSize64 > maxImportEntryBytes {
			skipped = append(skipped, importSkip{Name: base, Reason: "file too large"})
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			skipped = append(skipped, importSkip{Name: base, Reason: "cannot read entry"})
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(rc, maxImportEntryBytes))
		rc.Close()
		if err != nil {
			skipped = append(skipped, importSkip{Name: base, Reason: "cannot read entry"})
			continue
		}
		content := strings.TrimSpace(string(raw))
		var probe any
		if err := common.UnmarshalJsonStr(content, &probe); err != nil {
			skipped = append(skipped, importSkip{Name: base, Reason: "not valid JSON"})
			continue
		}
		// A single zip entry can itself be a multi-account bundle/array; expand it
		// so every account inside is imported, not just the first.
		items = append(items, parsePoolAuthAccounts(content, base)...)
	}

	imported, failed, forwardSkipped, err := forwardPoolAuthItems(c.Request.Context(), baseURL, secret, poolID, items)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	skipped = append(skipped, forwardSkipped...)

	recordManageAudit(c, "pool_auth.import", map[string]interface{}{
		"pool":     auditPoolID(poolID),
		"imported": imported,
		"skipped":  len(skipped),
		"failed":   len(failed),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"imported": imported,
			"skipped":  skipped,
			"failed":   failed,
		},
	})
}

// poolZipEntryBase returns the final path element of a zip entry name, treating
// both '/' and '\\' as separators (zip entries can carry either).
func poolZipEntryBase(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return name
}

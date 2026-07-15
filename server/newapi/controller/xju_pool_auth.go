package controller

import (
	"archive/zip"
	"bytes"
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

	// A pasted export can be a single codex auth object OR an exporter bundle
	// (`{accounts:[{credentials:{...}}]}`). Unwrap the bundle to the inner
	// credential the pool actually understands.
	content, name := unwrapPoolAuthContent(content, reqBody.Name)

	// xju-api:new — refuse a file that carries another pool's marker.
	if fp, misrouted := foreignPoolMarker(name, c.Query("pool")); misrouted {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": fmt.Sprintf("this file name contains \"-%s-\", so it looks like a %q pool account; switch to that pool (or rename) before adding", fp, fp),
		})
		return
	}

	ok := poolMgmtProxy(
		c, c.Query("pool"), http.MethodPost,
		"/v0/management/auth-files?name="+url.QueryEscape(name),
		strings.NewReader(content),
		"application/json",
	)
	if ok {
		// xju-api:edit — 审计对齐(对标 invite_code.go 的 recordManageAudit 规范):
		// 只记池与文件名,凭证内容永不入审计。
		recordManageAudit(c, "pool_auth.add", map[string]interface{}{
			"pool": auditPoolID(c.Query("pool")),
			"name": name,
		})
	}
}

// unwrapPoolAuthContent normalizes a pasted auth blob into (single-account JSON,
// filename). A bare codex object passes through; an exporter bundle's first
// account's `credentials` is extracted. Name is derived from the account email
// when the caller didn't supply one.
func unwrapPoolAuthContent(content, rawName string) (string, string) {
	var obj map[string]any
	if err := common.UnmarshalJsonStr(content, &obj); err != nil {
		return content, sanitizePoolAuthName(rawName)
	}

	// Exporter bundle: {exported_at, proxies, accounts:[{credentials:{...}}]}
	if accounts, ok := obj["accounts"].([]any); ok && len(accounts) > 0 {
		if first, ok := accounts[0].(map[string]any); ok {
			if creds, ok := first["credentials"].(map[string]any); ok {
				if inner, err := common.Marshal(creds); err == nil {
					n := rawName
					if n == "" {
						n = stringField(creds, "email")
					}
					return string(inner), sanitizePoolAuthName(n)
				}
			}
		}
	}

	// Single codex object — derive a readable name from its email if none given.
	n := rawName
	if n == "" {
		n = stringField(obj, "email")
	}
	return content, sanitizePoolAuthName(n)
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

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	skipped := make([]importSkip, 0)
	forwarded := 0
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
		// Reuse the single-add normalization: unwrap export bundles, derive a safe name.
		normalized, name := unwrapPoolAuthContent(content, base)
		// xju-api:new — skip files that belong to another pool (name carries its
		// marker), so a mis-targeted zip can't silently pollute this pool.
		if fp, misrouted := foreignPoolMarker(name, poolID); misrouted {
			skipped = append(skipped, importSkip{Name: base, Reason: "belongs to pool " + fp + " (name contains -" + fp + "-)"})
			continue
		}
		part, err := mw.CreateFormFile("files", name)
		if err != nil {
			skipped = append(skipped, importSkip{Name: base, Reason: "internal error"})
			continue
		}
		if _, err := part.Write([]byte(normalized)); err != nil {
			skipped = append(skipped, importSkip{Name: base, Reason: "internal error"})
			continue
		}
		forwarded++
	}
	if err := mw.Close(); err != nil {
		common.ApiError(c, err)
		return
	}

	failed := make([]importFail, 0)
	imported := 0
	if forwarded > 0 {
		status, payload, err := service.PoolMgmtRoundTrip(
			c.Request.Context(), baseURL, secret,
			http.MethodPost, "/v0/management/auth-files", &body, mw.FormDataContentType(),
		)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": fmt.Sprintf("pool management unreachable: %v", err)})
			return
		}
		if status < 200 || status >= 300 {
			c.JSON(status, gin.H{"success": false, "message": poolErrorMessage(payload, status)})
			return
		}
		// Pool response: {status, uploaded, files:[...], failed:[{name,error}]}
		var parsed struct {
			Uploaded int          `json:"uploaded"`
			Files    []string     `json:"files"`
			Failed   []importFail `json:"failed"`
		}
		if err := common.Unmarshal(payload, &parsed); err == nil {
			failed = append(failed, parsed.Failed...)
			imported = parsed.Uploaded
			if imported == 0 && len(parsed.Files) > 0 {
				imported = len(parsed.Files)
			}
			if imported == 0 && len(parsed.Failed) == 0 {
				imported = forwarded // all-ok response without an explicit count
			}
		} else {
			imported = forwarded
		}
	}

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

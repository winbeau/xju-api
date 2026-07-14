package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// Pool-authentication proxy.
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

func poolMgmtBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("POOL_MGMT_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://cli-proxy-api:8317"
}

func poolMgmtSecret() string {
	return strings.TrimSpace(os.Getenv("POOL_MGMT_SECRET"))
}

var poolMgmtClient = &http.Client{Timeout: 20 * time.Second}

// poolMgmtProxy forwards a request to the CLIProxyAPI management API, attaching
// the Bearer secret, and copies the upstream status + body back to the caller.
func poolMgmtProxy(c *gin.Context, method, path string, body io.Reader, contentType string) {
	secret := poolMgmtSecret()
	if secret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "pool management is not configured (POOL_MGMT_SECRET unset)",
		})
		return
	}

	req, err := http.NewRequestWithContext(
		c.Request.Context(), method, poolMgmtBaseURL()+path, body,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := poolMgmtClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": fmt.Sprintf("pool management unreachable: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// The management API returns its own JSON shape; wrap it so the frontend
	// gets the uniform {success, data|message} envelope it expects everywhere.
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	if ok {
		c.Data(http.StatusOK, "application/json; charset=utf-8", wrapPoolSuccess(payload))
		return
	}
	c.JSON(resp.StatusCode, gin.H{
		"success": false,
		"message": poolErrorMessage(payload, resp.StatusCode),
	})
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
	poolMgmtProxy(c, http.MethodGet, "/v0/management/auth-files", nil, "")
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

	poolMgmtProxy(
		c, http.MethodPost,
		"/v0/management/auth-files?name="+url.QueryEscape(name),
		strings.NewReader(content),
		"application/json",
	)
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
	body, err := common.Marshal(map[string]any{
		"name":     sanitizePoolAuthName(reqBody.Name),
		"disabled": *reqBody.Disabled,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	poolMgmtProxy(
		c, http.MethodPatch, "/v0/management/auth-files/status",
		strings.NewReader(string(body)), "application/json",
	)
}

// CleanPoolAuthFilesNow POST /api/pool/auth-files/clean — run the stale-account
// sweep on demand (same logic as the hourly auto-clean), using the current
// PoolAutoCleanHours threshold. Returns how many accounts were disabled.
func CleanPoolAuthFilesNow(c *gin.Context) {
	if poolMgmtSecret() == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "pool management is not configured (POOL_MGMT_SECRET unset)",
		})
		return
	}
	disabled, err := service.SweepPoolOnce(common.PoolAutoCleanHours)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"disabled": disabled}})
}

// DeletePoolAuthFile DELETE /api/pool/auth-files?name=xxx — remove one account.
func DeletePoolAuthFile(c *gin.Context) {
	name := sanitizePoolAuthName(c.Query("name"))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "name is required"})
		return
	}
	poolMgmtProxy(
		c, http.MethodDelete,
		"/v0/management/auth-files?name="+url.QueryEscape(name),
		nil, "",
	)
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

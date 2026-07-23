package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const privatePoolOAuthDefaultTTL = 30 * time.Minute

var privatePoolOAuthStatePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,256}$`)

type privatePoolOAuthStartResponse struct {
	Status    string `json:"status"`
	URL       string `json:"url"`
	State     string `json:"state"`
	ExpiresIn int    `json:"expires_in"`
}

type privatePoolOAuthCallbackRequest struct {
	SessionID   string `json:"session_id"`
	RedirectURL string `json:"redirect_url"`
}

type privatePoolOAuthStatusResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

func setPrivatePoolOAuthNoStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
}

func validCodexAuthorizationURL(raw, state string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	query := u.Query()
	return u.Scheme == "https" &&
		u.User == nil &&
		strings.EqualFold(u.Hostname(), "auth.openai.com") &&
		(u.Port() == "" || u.Port() == "443") &&
		u.Path == "/oauth/authorize" &&
		u.Fragment == "" &&
		query.Get("state") == strings.TrimSpace(state) &&
		query.Get("redirect_uri") == "http://localhost:1455/auth/callback"
}

func parseCodexCallbackURL(raw string) (code, state, errorMessage string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", "", err
	}
	if u.Scheme != "http" || u.User != nil || !strings.EqualFold(u.Hostname(), "localhost") || u.Port() != "1455" || u.Path != "/auth/callback" || u.Fragment != "" {
		return "", "", "", &url.Error{Op: "validate", URL: "Codex callback", Err: errInvalidCodexCallbackURL}
	}
	query := u.Query()
	code = strings.TrimSpace(query.Get("code"))
	state = strings.TrimSpace(query.Get("state"))
	errorMessage = strings.TrimSpace(query.Get("error"))
	if errorMessage == "" {
		errorMessage = strings.TrimSpace(query.Get("error_description"))
	}
	if state == "" || (code == "" && errorMessage == "") {
		return "", "", "", &url.Error{Op: "validate", URL: "Codex callback", Err: errInvalidCodexCallbackURL}
	}
	return code, state, errorMessage, nil
}

var errInvalidCodexCallbackURL = &privatePoolOAuthValidationError{"invalid Codex callback URL"}

type privatePoolOAuthValidationError struct{ message string }

func (e *privatePoolOAuthValidationError) Error() string { return e.message }

// StartPrivatePoolCodexOAuth creates a Codex OAuth flow inside the current
// user's CLIProxyAPI instance. It intentionally omits is_webui=1: the browser
// will return the localhost callback URL to this API instead of using SSH -L.
func StartPrivatePoolCodexOAuth(c *gin.Context) {
	setPrivatePoolOAuthNoStore(c)
	ownerUserID := c.GetInt("id")
	poolID := poolIDFromRequest(c)
	session, err := service.ReservePrivatePoolOAuthSession(ownerUserID, poolID, "codex", privatePoolOAuthDefaultTTL)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
		return
	}
	release := true
	defer func() {
		if release {
			service.DeletePrivatePoolOAuthSession(session.ID, ownerUserID)
		}
	}()

	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "private pool is not ready"})
		return
	}
	existing, err := privatePoolExistingAccountNames(c.Request.Context(), baseURL, secret)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	if len(existing)+service.CountPrivatePoolOAuthReservations(poolID) > privateMaxAccounts {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "private pool account limit is " + strconv.Itoa(privateMaxAccounts)})
		return
	}

	status, payload, err := service.PoolMgmtRoundTrip(c.Request.Context(), baseURL, secret, http.MethodGet, "/v0/management/codex-auth-url", nil, "")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "pool management unreachable"})
		return
	}
	if status < 200 || status >= 300 {
		c.JSON(status, gin.H{"success": false, "message": poolErrorMessage(payload, status)})
		return
	}
	var upstream privatePoolOAuthStartResponse
	if err := json.Unmarshal(payload, &upstream); err != nil || !privatePoolOAuthStatePattern.MatchString(strings.TrimSpace(upstream.State)) || !validCodexAuthorizationURL(upstream.URL, upstream.State) {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "pool returned an invalid OAuth session"})
		return
	}
	if upstream.ExpiresIn < 60 || upstream.ExpiresIn > 3600 {
		upstream.ExpiresIn = int(privatePoolOAuthDefaultTTL / time.Second)
	}
	activated, err := service.ActivatePrivatePoolOAuthSession(session.ID, upstream.State, time.Now().Add(time.Duration(upstream.ExpiresIn)*time.Second))
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
		return
	}
	release = false
	recordPoolAudit(c, "private_pool.oauth_start", map[string]interface{}{"pool": auditPoolID(poolID), "provider": "codex"})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"session_id": activated.ID,
		"status":     activated.Phase,
		"url":        upstream.URL,
		"expires_in": upstream.ExpiresIn,
		"expires_at": activated.ExpiresAt.Unix(),
	}})
}

// SubmitPrivatePoolCodexOAuthCallback accepts only the registered localhost
// callback shape, extracts its one-time code, verifies state ownership, and
// immediately forwards it to the owner's pool without persisting the URL/code.
func SubmitPrivatePoolCodexOAuthCallback(c *gin.Context) {
	setPrivatePoolOAuthNoStore(c)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var req privatePoolOAuthCallbackRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	ownerUserID := c.GetInt("id")
	session, ok := service.GetPrivatePoolOAuthSession(req.SessionID, ownerUserID)
	if !ok || session.Provider != "codex" || session.UpstreamState == "" {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "login session expired"})
		return
	}
	code, state, errorMessage, err := parseCodexCallbackURL(req.RedirectURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "paste the complete localhost:1455 callback URL"})
		return
	}
	if state != session.UpstreamState {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "OAuth state does not match this login session"})
		return
	}
	body, _ := json.Marshal(gin.H{"provider": "codex", "state": state, "code": code, "error": errorMessage})
	baseURL, secret, ready := common.ResolvePoolMgmt(session.PoolID)
	if !ready {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "private pool is not ready"})
		return
	}
	status, payload, err := service.PoolMgmtRoundTrip(c.Request.Context(), baseURL, secret, http.MethodPost, "/v0/management/oauth-callback", bytes.NewReader(body), "application/json")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "pool management unreachable"})
		return
	}
	if status < 200 || status >= 300 {
		c.JSON(status, gin.H{"success": false, "message": poolErrorMessage(payload, status)})
		return
	}
	updated, err := service.MarkPrivatePoolOAuthCallbackSubmitted(session.ID, ownerUserID)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordPoolAudit(c, "private_pool.oauth_callback", map[string]interface{}{"pool": auditPoolID(session.PoolID), "provider": "codex"})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"session_id": updated.ID, "status": updated.Phase}})
}

func GetPrivatePoolCodexOAuthStatus(c *gin.Context) {
	setPrivatePoolOAuthNoStore(c)
	ownerUserID := c.GetInt("id")
	sessionID := strings.TrimSpace(c.Query("session_id"))
	session, ok := service.GetPrivatePoolOAuthSession(sessionID, ownerUserID)
	if !ok || session.Provider != "codex" {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "login session expired"})
		return
	}
	baseURL, secret, ready := common.ResolvePoolMgmt(session.PoolID)
	if !ready {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "private pool is not ready"})
		return
	}
	path := "/v0/management/get-auth-status?state=" + url.QueryEscape(session.UpstreamState)
	status, payload, err := service.PoolMgmtRoundTrip(c.Request.Context(), baseURL, secret, http.MethodGet, path, nil, "")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "pool management unreachable"})
		return
	}
	if status < 200 || status >= 300 {
		c.JSON(status, gin.H{"success": false, "message": poolErrorMessage(payload, status)})
		return
	}
	var upstream privatePoolOAuthStatusResponse
	if err := json.Unmarshal(payload, &upstream); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "pool returned an invalid OAuth status"})
		return
	}
	switch upstream.Status {
	case "ok":
		service.DeletePrivatePoolOAuthSession(session.ID, ownerUserID)
		recordPoolAudit(c, "private_pool.oauth_complete", map[string]interface{}{"pool": auditPoolID(session.PoolID), "provider": "codex"})
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "ok"}})
	case "error":
		service.DeletePrivatePoolOAuthSession(session.ID, ownerUserID)
		message := strings.TrimSpace(upstream.Error)
		if message == "" {
			message = "authentication failed"
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "error", "error": message}})
	default:
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": session.Phase}})
	}
}

func CancelPrivatePoolCodexOAuth(c *gin.Context) {
	setPrivatePoolOAuthNoStore(c)
	ownerUserID := c.GetInt("id")
	sessionID := strings.TrimSpace(c.Query("session_id"))
	session, ok := service.GetPrivatePoolOAuthSession(sessionID, ownerUserID)
	if !ok || session.Provider != "codex" {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "cancelled"}})
		return
	}
	if baseURL, secret, ready := common.ResolvePoolMgmt(session.PoolID); ready && session.UpstreamState != "" {
		path := "/v0/management/oauth-session?state=" + url.QueryEscape(session.UpstreamState)
		_, _, _ = service.PoolMgmtRoundTrip(c.Request.Context(), baseURL, secret, http.MethodDelete, path, nil, "")
	}
	service.DeletePrivatePoolOAuthSession(session.ID, ownerUserID)
	recordPoolAudit(c, "private_pool.oauth_cancel", map[string]interface{}{"pool": auditPoolID(session.PoolID), "provider": "codex"})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "cancelled"}})
}

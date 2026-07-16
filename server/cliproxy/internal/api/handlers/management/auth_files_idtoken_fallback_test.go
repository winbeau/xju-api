package management

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// codexJWT builds an unsigned JWT whose payload carries the OpenAI auth claim.
// ParseJWTToken does not verify signatures, so a placeholder signature is fine.
func codexJWT(t *testing.T, plan, accountID, subUntil string) string {
	t.Helper()
	auth := map[string]any{}
	if plan != "" {
		auth["chatgpt_plan_type"] = plan
	}
	if accountID != "" {
		auth["chatgpt_account_id"] = accountID
	}
	if subUntil != "" {
		auth["chatgpt_subscription_active_until"] = subUntil
	}
	payload, err := json.Marshal(map[string]any{"https://api.openai.com/auth": auth})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func TestExtractCodexIDTokenClaims_FallsBackToAccessToken(t *testing.T) {
	// A refreshed codex account has an empty id_token but a valid access_token
	// carrying the same plan/subscription claim. The plan badge must still show.
	auth := &coreauth.Auth{
		Provider: "codex",
		Metadata: map[string]any{
			"type":         "codex",
			"id_token":     "",
			"access_token": codexJWT(t, "pro", "acc-refreshed", "2027-06-01T00:00:00Z"),
		},
	}
	claims := extractCodexIDTokenClaims(auth)
	if claims == nil {
		t.Fatalf("expected claims from access_token fallback, got nil")
	}
	if got := claims["plan_type"]; got != "pro" {
		t.Fatalf("expected plan_type pro from access_token, got %#v", got)
	}
	if got := claims["chatgpt_account_id"]; got != "acc-refreshed" {
		t.Fatalf("expected account id from access_token, got %#v", got)
	}
	if got := claims["chatgpt_subscription_active_until"]; got != "2027-06-01T00:00:00Z" {
		t.Fatalf("expected subscription window from access_token, got %#v", got)
	}
}

func TestExtractCodexIDTokenClaims_PrefersIDToken(t *testing.T) {
	// When both tokens are present the id_token wins (it is the canonical source).
	auth := &coreauth.Auth{
		Provider: "codex",
		Metadata: map[string]any{
			"type":         "codex",
			"id_token":     codexJWT(t, "plus", "acc-id", ""),
			"access_token": codexJWT(t, "pro", "acc-access", ""),
		},
	}
	claims := extractCodexIDTokenClaims(auth)
	if claims == nil {
		t.Fatalf("expected claims, got nil")
	}
	if got := claims["plan_type"]; got != "plus" {
		t.Fatalf("expected id_token plan_type plus to win, got %#v", got)
	}
}

func TestExtractCodexIDTokenClaims_NoTokensReturnsNil(t *testing.T) {
	auth := &coreauth.Auth{
		Provider: "codex",
		Metadata: map[string]any{"type": "codex"},
	}
	if claims := extractCodexIDTokenClaims(auth); claims != nil {
		t.Fatalf("expected nil claims when no token present, got %#v", claims)
	}
}

func TestExtractCodexIDTokenClaims_TopLevelPlanFallback(t *testing.T) {
	// A genuinely-lean go-pool account has no id_token/access_token JWT. The plan
	// survives only as a top-level metadata field; the subscription date exists
	// nowhere. The plan badge must still light up.
	auth := &coreauth.Auth{
		Provider: "codex",
		Metadata: map[string]any{
			"type":               "codex",
			"chatgpt_plan_type":  "plus",
			"chatgpt_account_id": "acc-lean",
		},
	}
	claims := extractCodexIDTokenClaims(auth)
	if claims == nil {
		t.Fatalf("expected claims from top-level plan fallback, got nil")
	}
	if got := claims["plan_type"]; got != "plus" {
		t.Fatalf("expected top-level plan_type plus, got %#v", got)
	}
	if got := claims["chatgpt_account_id"]; got != "acc-lean" {
		t.Fatalf("expected top-level account id, got %#v", got)
	}
	if _, ok := claims["chatgpt_subscription_active_until"]; ok {
		t.Fatalf("lean account must not report a subscription window, got %#v", claims["chatgpt_subscription_active_until"])
	}
}

func TestExtractCodexIDTokenClaims_JWTPlanBeatsTopLevel(t *testing.T) {
	// When both a JWT plan and a divergent top-level plan exist, the JWT wins.
	auth := &coreauth.Auth{
		Provider: "codex",
		Metadata: map[string]any{
			"type":              "codex",
			"id_token":          codexJWT(t, "plus", "acc-id", ""),
			"chatgpt_plan_type": "pro",
		},
	}
	claims := extractCodexIDTokenClaims(auth)
	if claims == nil {
		t.Fatalf("expected claims, got nil")
	}
	if got := claims["plan_type"]; got != "plus" {
		t.Fatalf("expected JWT plan_type plus to beat top-level pro, got %#v", got)
	}
}

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// xju-api:new — the probe classification table (号池验活 Part A). Every row is
// an empirically-confirmed signal from the codex /responses endpoint (see the
// tri probe log in the design): 405=alive, 401/402=dead, 429+usage_limit=quota,
// 429 else=rate, 2xx (heavy)=alive.

func TestClassifyProbe(t *testing.T) {
	cases := []struct {
		name        string
		subExpired  bool
		code        int
		body        string
		wantVerdict ProbeVerdict
	}{
		{"subscription expired short-circuits before any call", true, 0, "", VerdictSubscriptionExpired},
		{"subscription check wins even over a live 405", true, 405, "", VerdictSubscriptionExpired},
		{"light GET 405 = token live", false, 405, `{"detail":"Method Not Allowed"}`, VerdictOnline},
		{"heavy POST 200 = can infer", false, 200, "event: response.created", VerdictOnline},
		{"401 = dead token", false, 401, `{"error":{"code":"unauthorized_unknown"}}`, VerdictCredentialDead},
		{"402 deactivated workspace = dead account", false, 402, `{"detail":{"code":"deactivated_workspace"}}`, VerdictCredentialDead},
		{"403 forbidden = dead account", false, 403, "forbidden", VerdictCredentialDead},
		{"429 usage_limit_reached = quota exhausted", false, 429, `{"error":{"type":"usage_limit_reached"}}`, VerdictQuotaExhausted},
		{"429 plain = transient rate limit", false, 429, `{"error":{"type":"rate_limit_exceeded"}}`, VerdictRateLimited},
		{"transport failure (code 0) = unknown", false, 0, "", VerdictUnknown},
		{"5xx = unknown", false, 503, "bad gateway", VerdictUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, detail := classifyProbe(tc.subExpired, tc.code, tc.body)
			assert.Equal(t, tc.wantVerdict, v)
			if v != VerdictOnline {
				assert.NotEmpty(t, detail, "non-online verdicts should carry a detail")
			}
		})
	}
}

func TestProbeVerdict_Dead(t *testing.T) {
	// Only genuinely-dead verdicts are auto-disable eligible; transient and
	// unknown states must never be auto-disabled.
	assert.True(t, VerdictCredentialDead.dead())
	assert.True(t, VerdictSubscriptionExpired.dead())
	assert.False(t, VerdictRateLimited.dead())
	assert.False(t, VerdictQuotaExhausted.dead())
	assert.False(t, VerdictOnline.dead())
	assert.False(t, VerdictUnknown.dead())
}

func TestProbeJobSnapshotResultsNeverNil(t *testing.T) {
	// An empty job (just started / all accounts disabled) must snapshot Results
	// as a non-nil slice so it marshals as [] not null — the frontend iterates
	// it and reads .length, and a null crashes the pool page.
	j := &probeJob{}
	snap := j.Snapshot()
	assert.NotNil(t, snap.Results)
	assert.Len(t, snap.Results, 0)
}

func TestSubscriptionExpiredAt(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	assert.True(t, subscriptionExpiredAt("2026-07-01T00:00:00Z", now))
	assert.False(t, subscriptionExpiredAt("2027-01-01T00:00:00Z", now))
	assert.False(t, subscriptionExpiredAt("", now))
	assert.False(t, subscriptionExpiredAt("garbage", now))
}

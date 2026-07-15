package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// xju-api:new — the subscription-expired sweep judgment (REFACTOR-PLAN Part C).
// A closed ChatGPT subscription window is a certain death the sweep disables
// directly; a future window or a non-codex account (no id_token) must not be
// touched by this rule.

func TestPoolAuthEntry_SubscriptionExpired(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	mk := func(until string) poolAuthEntry {
		var e poolAuthEntry
		e.IDToken.SubscriptionActiveUntil = until
		return e
	}

	cases := []struct {
		name  string
		until string
		want  bool
	}{
		{"past window → expired", "2026-07-14T12:00:00Z", true},
		{"future window → alive", "2026-08-12T10:12:19+00:00", false},
		{"empty (non-codex) → alive", "", false},
		{"unparseable → alive (don't disable on bad data)", "not-a-date", false},
		{"exactly now → not yet expired", "2026-07-15T12:00:00Z", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, mk(tc.until).subscriptionExpired(now))
		})
	}
}

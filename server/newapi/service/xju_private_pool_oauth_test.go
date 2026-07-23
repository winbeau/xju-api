package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivatePoolOAuthSessionOwnershipAndReservation(t *testing.T) {
	owner := 78001
	poolID := "private-oauth-test"
	session, err := ReservePrivatePoolOAuthSession(owner, poolID, "codex", time.Minute)
	require.NoError(t, err)
	t.Cleanup(func() { DeletePrivatePoolOAuthSession(session.ID, owner) })
	assert.Equal(t, 1, CountPrivatePoolOAuthReservations(poolID))

	_, err = ReservePrivatePoolOAuthSession(owner, poolID, "codex", time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already in progress")

	_, ok := GetPrivatePoolOAuthSession(session.ID, owner+1)
	assert.False(t, ok)
	activated, err := ActivatePrivatePoolOAuthSession(session.ID, "0123456789abcdef0123456789abcdef", time.Now().Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, "waiting_callback", activated.Phase)

	updated, err := MarkPrivatePoolOAuthCallbackSubmitted(session.ID, owner)
	require.NoError(t, err)
	assert.Equal(t, "exchanging", updated.Phase)

	DeletePrivatePoolOAuthSession(session.ID, owner)
	assert.Equal(t, 0, CountPrivatePoolOAuthReservations(poolID))
}

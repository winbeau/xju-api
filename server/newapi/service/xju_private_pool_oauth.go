package service

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// PrivatePoolOAuthSession binds a short-lived upstream OAuth state to the
// authenticated owner of one private pool. The browser receives only ID; the
// upstream state never becomes an authority for selecting a pool.
type PrivatePoolOAuthSession struct {
	ID            string
	OwnerUserID   int
	PoolID        string
	Provider      string
	UpstreamState string
	Phase         string
	ExpiresAt     time.Time
}

var privatePoolOAuthSessions = struct {
	sync.Mutex
	byID    map[string]PrivatePoolOAuthSession
	byOwner map[int]string
}{
	byID:    make(map[string]PrivatePoolOAuthSession),
	byOwner: make(map[int]string),
}

func cleanupPrivatePoolOAuthSessionsLocked(now time.Time) {
	for id, session := range privatePoolOAuthSessions.byID {
		if now.Before(session.ExpiresAt) {
			continue
		}
		delete(privatePoolOAuthSessions.byID, id)
		if privatePoolOAuthSessions.byOwner[session.OwnerUserID] == id {
			delete(privatePoolOAuthSessions.byOwner, session.OwnerUserID)
		}
	}
}

// ReservePrivatePoolOAuthSession permits at most one active login per owner.
// Reserving before the upstream call also reserves one private-pool account
// slot against concurrent JSON/ZIP imports.
func ReservePrivatePoolOAuthSession(ownerUserID int, poolID, provider string, ttl time.Duration) (PrivatePoolOAuthSession, error) {
	poolID = strings.TrimSpace(poolID)
	provider = strings.ToLower(strings.TrimSpace(provider))
	if ownerUserID <= 0 || poolID == "" || provider == "" {
		return PrivatePoolOAuthSession{}, fmt.Errorf("invalid private pool OAuth session")
	}
	if ttl <= 0 || ttl > time.Hour {
		ttl = 30 * time.Minute
	}

	privatePoolOAuthSessions.Lock()
	defer privatePoolOAuthSessions.Unlock()
	cleanupPrivatePoolOAuthSessionsLocked(time.Now())
	if existing := privatePoolOAuthSessions.byOwner[ownerUserID]; existing != "" {
		return PrivatePoolOAuthSession{}, fmt.Errorf("an account login is already in progress")
	}

	session := PrivatePoolOAuthSession{
		ID:          uuid.NewString(),
		OwnerUserID: ownerUserID,
		PoolID:      poolID,
		Provider:    provider,
		Phase:       "starting",
		ExpiresAt:   time.Now().Add(ttl),
	}
	privatePoolOAuthSessions.byID[session.ID] = session
	privatePoolOAuthSessions.byOwner[ownerUserID] = session.ID
	return session, nil
}

func ActivatePrivatePoolOAuthSession(sessionID, upstreamState string, expiresAt time.Time) (PrivatePoolOAuthSession, error) {
	privatePoolOAuthSessions.Lock()
	defer privatePoolOAuthSessions.Unlock()
	cleanupPrivatePoolOAuthSessionsLocked(time.Now())
	session, ok := privatePoolOAuthSessions.byID[strings.TrimSpace(sessionID)]
	if !ok {
		return PrivatePoolOAuthSession{}, fmt.Errorf("login session expired")
	}
	upstreamState = strings.TrimSpace(upstreamState)
	if upstreamState == "" {
		return PrivatePoolOAuthSession{}, fmt.Errorf("upstream OAuth state is missing")
	}
	session.UpstreamState = upstreamState
	session.Phase = "waiting_callback"
	if expiresAt.After(time.Now()) && expiresAt.Before(time.Now().Add(time.Hour)) {
		session.ExpiresAt = expiresAt
	}
	privatePoolOAuthSessions.byID[session.ID] = session
	return session, nil
}

func GetPrivatePoolOAuthSession(sessionID string, ownerUserID int) (PrivatePoolOAuthSession, bool) {
	privatePoolOAuthSessions.Lock()
	defer privatePoolOAuthSessions.Unlock()
	cleanupPrivatePoolOAuthSessionsLocked(time.Now())
	session, ok := privatePoolOAuthSessions.byID[strings.TrimSpace(sessionID)]
	if !ok || session.OwnerUserID != ownerUserID {
		return PrivatePoolOAuthSession{}, false
	}
	return session, true
}

func MarkPrivatePoolOAuthCallbackSubmitted(sessionID string, ownerUserID int) (PrivatePoolOAuthSession, error) {
	privatePoolOAuthSessions.Lock()
	defer privatePoolOAuthSessions.Unlock()
	cleanupPrivatePoolOAuthSessionsLocked(time.Now())
	session, ok := privatePoolOAuthSessions.byID[strings.TrimSpace(sessionID)]
	if !ok || session.OwnerUserID != ownerUserID {
		return PrivatePoolOAuthSession{}, fmt.Errorf("login session expired")
	}
	session.Phase = "exchanging"
	privatePoolOAuthSessions.byID[session.ID] = session
	return session, nil
}

func DeletePrivatePoolOAuthSession(sessionID string, ownerUserID int) {
	privatePoolOAuthSessions.Lock()
	defer privatePoolOAuthSessions.Unlock()
	id := strings.TrimSpace(sessionID)
	session, ok := privatePoolOAuthSessions.byID[id]
	if !ok || session.OwnerUserID != ownerUserID {
		return
	}
	delete(privatePoolOAuthSessions.byID, id)
	if privatePoolOAuthSessions.byOwner[ownerUserID] == id {
		delete(privatePoolOAuthSessions.byOwner, ownerUserID)
	}
}

func CountPrivatePoolOAuthReservations(poolID string) int {
	privatePoolOAuthSessions.Lock()
	defer privatePoolOAuthSessions.Unlock()
	cleanupPrivatePoolOAuthSessionsLocked(time.Now())
	count := 0
	for _, session := range privatePoolOAuthSessions.byID {
		if session.PoolID == poolID {
			count++
		}
	}
	return count
}

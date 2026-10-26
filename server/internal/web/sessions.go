package web

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

// sessionTTL is the absolute session lifetime; there is no sliding renewal [AD-8].
const sessionTTL = 8 * time.Hour

// session is one signed-in browser. It keeps the token's hash, never the token.
type session struct {
	id, tokenHash, csrf string
	created             time.Time
}

// sessions is the in-memory session store; a restart signs everybody out [AD-8].
type sessions struct {
	mu   sync.Mutex
	byID map[string]session
	now  func() time.Time
}

func newSessions(now func() time.Time) *sessions {
	return &sessions{byID: map[string]session{}, now: now}
}

// create stores a new session for tokenHash with a fresh opaque id and CSRF token.
func (s *sessions) create(tokenHash string) (session, error) {
	id, err := randomID()
	if err != nil {
		return session{}, err
	}
	csrf, err := randomID()
	if err != nil {
		return session{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.dropExpired(now)
	sess := session{id: id, tokenHash: tokenHash, csrf: csrf, created: now}
	s.byID[id] = sess
	return sess, nil
}

// get returns the live session with id; an expired session is dropped.
func (s *sessions) get(id string) (session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.byID[id]
	if ok && sess.expired(s.now()) {
		delete(s.byID, id)
		return session{}, false
	}
	return sess, ok
}

// drop ends the session with id, if any.
func (s *sessions) drop(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, id)
}

// dropExpired removes every expired session. Call with mu held.
func (s *sessions) dropExpired(now time.Time) {
	for id, sess := range s.byID {
		if sess.expired(now) {
			delete(s.byID, id)
		}
	}
}

func (sess session) expired(now time.Time) bool {
	return !now.Before(sess.created.Add(sessionTTL))
}

// randomID returns 32 random bytes, RawURL base64 encoded (43 characters).
func randomID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

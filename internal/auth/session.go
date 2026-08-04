// Package auth issues and validates the "limited sessions" an installed
// application receives (vision document, section 15.4): a session is
// scoped to exactly the permissions its application's manifest declares
// and never carries the rest of the public API's default access.
//
// This is the minimal first slice (ADR-0026): sessions live only in
// memory, are never persisted, never expire, and cannot be revoked before
// the agent restarts. That was an explicit, temporary scope cut when
// written, on the grounds that there was no admin-level authentication
// anywhere else in the agent either — ADR-0036 has since added one, so a
// session's blast radius is now bounded by its own permissions (never more
// than one application's declared workflows_run) rather than by "nothing
// else is protected today either". Expiry/revocation before restart remain
// unimplemented; a leaked session token is a smaller, scoped risk than a
// leaked admin token, not a nonexistent one.
package auth

import (
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/lucasglmt/patchcord/internal/apps"
)

// ErrInvalidSession is returned by Store.Validate when the token is
// unknown.
var ErrInvalidSession = errors.New("invalid app session")

// Session is a credential granted to one installed application, limited to
// that application's declared permissions.
type Session struct {
	Token       string
	AppID       string
	Permissions apps.AppPermissions
	IssuedAt    time.Time
}

// CanRunWorkflow reports whether this session's application declared
// permission to run the workflow identified by id.
func (s Session) CanRunWorkflow(id string) bool {
	return slices.Contains(s.Permissions.WorkflowsRun, id)
}

// Store issues and validates sessions, keyed by their token, entirely in
// memory.
type Store struct {
	mu       sync.Mutex
	sessions map[string]Session
}

// NewStore returns an empty session store.
func NewStore() *Store {
	return &Store{sessions: make(map[string]Session)}
}

// Issue creates and records a new session limited to app's declared
// permissions.
func (s *Store) Issue(app apps.App) Session {
	session := Session{
		Token:       uuid.NewString(),
		AppID:       app.ID,
		Permissions: app.Permissions,
		IssuedAt:    time.Now().UTC(),
	}

	s.mu.Lock()
	s.sessions[session.Token] = session
	s.mu.Unlock()

	return session
}

// Validate returns the session recorded for token. It returns
// ErrInvalidSession if token is unknown.
func (s *Store) Validate(token string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[token]
	if !ok {
		return Session{}, ErrInvalidSession
	}
	return session, nil
}

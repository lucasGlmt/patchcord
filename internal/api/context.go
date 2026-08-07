package api

import (
	"context"
	"net/http"

	"github.com/lucasglmt/patchcord/internal/auth"
)

// sessionContextKey is the unexported type of the context key withSession
// stores an app session under — unexported so no other package can collide
// with or read it directly, only sessionFromContext can.
type sessionContextKey struct{}

// withSession returns a shallow copy of r whose context carries session,
// for a downstream handler (startRunAndRespond, ADR-0071) to read back via
// sessionFromContext. Only withRunAuth calls this, and only once it has
// already validated session via appSessionAllowsRun — this is not itself an
// authentication check.
func withSession(r *http.Request, session auth.Session) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), sessionContextKey{}, session))
}

// sessionFromContext returns the app session withSession stashed on ctx, if
// any. A request that reached its handler via an admin token, or before any
// admin token existed and carried no bearer token at all, has none — ok is
// false and callers should treat the caller as unrestricted, exactly as
// they did before sessions were threaded through the context.
func sessionFromContext(ctx context.Context) (auth.Session, bool) {
	session, ok := ctx.Value(sessionContextKey{}).(auth.Session)
	return session, ok
}

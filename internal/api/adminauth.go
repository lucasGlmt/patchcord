package api

import (
	"errors"
	"net/http"

	"github.com/lucasglmt/patchcord/internal/auth"
)

// withAdminAuth requires a valid admin token ("Authorization: Bearer
// <token>", checked against auth.ValidateToken) once at least one has been
// created (auth.AnyTokensExist) — until then, every request reaches next
// unchanged, exactly as before admin tokens existed. This is an opt-in
// flipped by data (does a token exist), not by which address `patchcord
// serve` binds to: CLAUDE.md's non-negotiable #2 forbids branching core
// behavior on local-vs-server deployment, so binding beyond 127.0.0.1
// cannot be what decides this. See ADR-0036.
func withAdminAuth(deps Deps, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		enforced, err := auth.AnyTokensExist(r.Context(), deps.DB)
		if err != nil {
			http.Error(w, "admin auth: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !enforced {
			next(w, r)
			return
		}

		token, ok := bearerToken(r)
		if !ok {
			http.Error(w, "admin auth: missing bearer token", http.StatusUnauthorized)
			return
		}
		if _, err := auth.ValidateToken(r.Context(), deps.DB, token); err != nil {
			http.Error(w, "admin auth: "+err.Error(), http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// withRunAuth protects POST /workflows/{id}/run, the one route both an
// operator and an installed application legitimately call. While no admin
// token has been created yet, it behaves exactly like ADR-0026's original
// "optional app session" check: a bearer token, if present, must be a valid
// session scoped to this workflow; absent, the request proceeds
// unauthenticated. Once at least one admin token exists (ADR-0036), a
// bearer token is required, and is accepted either as an admin token (full
// access, like every other admin-gated route) or as a scoped app session —
// an installed application keeps working with only its own session, never
// needing an admin token of its own.
func withRunAuth(deps Deps, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		enforced, err := auth.AnyTokensExist(r.Context(), deps.DB)
		if err != nil {
			http.Error(w, "admin auth: "+err.Error(), http.StatusInternalServerError)
			return
		}

		token, hasToken := bearerToken(r)

		if !enforced {
			if !hasToken {
				next(w, r)
				return
			}
			if ok, status, msg := appSessionAllowsRun(deps, r, token); !ok {
				http.Error(w, msg, status)
				return
			}
			next(w, r)
			return
		}

		if !hasToken {
			http.Error(w, "admin auth: missing bearer token", http.StatusUnauthorized)
			return
		}
		if _, err := auth.ValidateToken(r.Context(), deps.DB, token); err == nil {
			next(w, r)
			return
		} else if !errors.Is(err, auth.ErrInvalidToken) {
			http.Error(w, "admin auth: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if ok, status, msg := appSessionAllowsRun(deps, r, token); !ok {
			http.Error(w, msg, status)
			return
		}
		next(w, r)
	}
}

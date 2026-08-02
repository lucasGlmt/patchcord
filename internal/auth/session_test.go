package auth

import (
	"errors"
	"testing"

	"github.com/lucasglmt/patchcord/internal/apps"
)

func TestStore_IssueAndValidate(t *testing.T) {
	store := NewStore()
	app := apps.App{
		ID:          "dashboard",
		Permissions: apps.AppPermissions{WorkflowsRun: []string{"hello_patchcord"}},
	}

	session := store.Issue(app)
	if session.Token == "" {
		t.Fatal("Issue() returned an empty token")
	}
	if session.AppID != "dashboard" {
		t.Fatalf("AppID = %q, want %q", session.AppID, "dashboard")
	}
	if session.IssuedAt.IsZero() {
		t.Fatal("IssuedAt is zero, want it populated")
	}

	got, err := store.Validate(session.Token)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got.AppID != "dashboard" {
		t.Fatalf("Validate() AppID = %q, want %q", got.AppID, "dashboard")
	}
}

func TestStore_Validate_RejectsAnUnknownToken(t *testing.T) {
	store := NewStore()

	if _, err := store.Validate("does-not-exist"); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("Validate() error = %v, want ErrInvalidSession", err)
	}
}

func TestStore_Issue_GeneratesDistinctTokens(t *testing.T) {
	store := NewStore()
	app := apps.App{ID: "dashboard"}

	first := store.Issue(app)
	second := store.Issue(app)

	if first.Token == second.Token {
		t.Fatalf("Issue() returned the same token twice: %q", first.Token)
	}
}

func TestSession_CanRunWorkflow(t *testing.T) {
	session := Session{
		Permissions: apps.AppPermissions{WorkflowsRun: []string{"hello_patchcord", "greet_twice"}},
	}

	if !session.CanRunWorkflow("hello_patchcord") {
		t.Fatal("CanRunWorkflow(hello_patchcord) = false, want true")
	}
	if session.CanRunWorkflow("ai_generate_text_demo") {
		t.Fatal("CanRunWorkflow(ai_generate_text_demo) = true, want false")
	}
}

func TestSession_CanRunWorkflow_EmptyPermissionsDenyEverything(t *testing.T) {
	session := Session{}

	if session.CanRunWorkflow("hello_patchcord") {
		t.Fatal("CanRunWorkflow() = true for a session with no declared permissions, want false")
	}
}

package skill

import (
	"context"
	"testing"

	"golang.org/x/oauth2"
)

// fakeTokenSource returns a fixed token without touching the network.
type fakeTokenSource struct{ tok string }

func (f fakeTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: f.tok}, nil
}

func TestGoogleAccessToken(t *testing.T) {
	got, err := googleAccessToken(fakeTokenSource{tok: "ya29.fake"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "ya29.fake" {
		t.Errorf("got %q", got)
	}
}

func TestGoogleTokenSourceErrors(t *testing.T) {
	if _, err := googleTokenSource(context.Background(), nil); err == nil {
		t.Error("nil config should error")
	}
	if _, err := googleTokenSource(context.Background(), &GoogleConfig{}); err == nil {
		t.Error("empty config should error")
	}
	// Refresh mode with unset env vars should error clearly.
	cfg := &GoogleConfig{Refresh: &GoogleRefresh{ClientEnv: "NOPE_CLIENT", SecretEnv: "NOPE_SECRET", RefreshEnv: "NOPE_REFRESH"}}
	if _, err := googleTokenSource(context.Background(), cfg); err == nil {
		t.Error("refresh with missing env should error")
	}
}

func TestResolveGoogleTokenNoAuth(t *testing.T) {
	// No auth configured -> empty token, no error (allows link-shared sheets).
	tok, err := resolveGoogleToken(nil)
	if err != nil || tok != "" {
		t.Errorf("no-auth: tok=%q err=%v", tok, err)
	}
	tok, err = resolveGoogleToken(&GoogleConfig{})
	if err != nil || tok != "" {
		t.Errorf("empty-auth: tok=%q err=%v", tok, err)
	}
}

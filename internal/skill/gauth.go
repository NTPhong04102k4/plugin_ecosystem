package skill

// Google auth for `sr fetch`. Mints a short-lived OAuth2 access token from either
// a service-account key file or a refresh token, using golang.org/x/oauth2 (which
// handles JWT signing / refresh and caches the token by TTL under the hood). The
// token is used as a Bearer credential against the Sheets CSV export / API.
// See docs/sr-fetch-design.md §7.

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// sheetsScope is read-only Sheets access — `sr fetch` never writes back.
const sheetsScope = "https://www.googleapis.com/auth/spreadsheets.readonly"

// googleTokenSource builds an oauth2.TokenSource from the config + environment.
// Resolution: a service-account key file wins if set, else refresh-token creds.
// The returned source caches and auto-refreshes access tokens internally.
func googleTokenSource(ctx context.Context, cfg *GoogleConfig) (oauth2.TokenSource, error) {
	if cfg == nil {
		return nil, fmt.Errorf("no google auth configured; set google.saKeyFile or google.refresh in .skillrunner/fetch.json")
	}

	if cfg.SAKeyFile != "" {
		keyPath := expandHome(cfg.SAKeyFile)
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("read service-account key %q: %w", keyPath, err)
		}
		jwtCfg, err := google.JWTConfigFromJSON(key, sheetsScope)
		if err != nil {
			return nil, fmt.Errorf("parse service-account key: %w", err)
		}
		return jwtCfg.TokenSource(ctx), nil
	}

	if cfg.Refresh != nil {
		clientID := os.Getenv(cfg.Refresh.ClientEnv)
		clientSecret := os.Getenv(cfg.Refresh.SecretEnv)
		refreshToken := os.Getenv(cfg.Refresh.RefreshEnv)
		if clientID == "" || clientSecret == "" || refreshToken == "" {
			return nil, fmt.Errorf("google refresh auth incomplete: set env vars %s / %s / %s",
				cfg.Refresh.ClientEnv, cfg.Refresh.SecretEnv, cfg.Refresh.RefreshEnv)
		}
		oauthCfg := &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     google.Endpoint,
			Scopes:       []string{sheetsScope},
		}
		return oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken}), nil
	}

	return nil, fmt.Errorf("google auth configured but empty; set google.saKeyFile or google.refresh")
}

// googleAccessToken resolves an access token string from a token source. Split
// out so tests can inject a fake source without hitting the network.
func googleAccessToken(ts oauth2.TokenSource) (string, error) {
	tok, err := ts.Token()
	if err != nil {
		return "", fmt.Errorf("obtain google access token (check service-account/refresh config): %w", err)
	}
	return tok.AccessToken, nil
}

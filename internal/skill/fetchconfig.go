package skill

// Config + secret resolution for `sr fetch`. The config lives at
// <project>/.skillrunner/fetch.json (gitignored) and holds only REFERENCES to
// secrets (env var names, a service-account key file path) plus source aliases —
// never raw tokens. Resolution order everywhere is: flag > env > config file.
// See docs/sr-fetch-design.md §7.

import (
	"os"
	"path/filepath"
)

// FetchConfig is the parsed .skillrunner/fetch.json (all fields optional).
type FetchConfig struct {
	Confluence *ConfluenceConfig `json:"confluence,omitempty"`
	Google     *GoogleConfig     `json:"google,omitempty"`
	// Sources maps a short alias to a full Confluence/Sheet URL, so callers can
	// `sr fetch --from tc-login` instead of pasting a long URL every time.
	Sources map[string]string `json:"sources,omitempty"`
}

// ConfluenceConfig holds the site + Basic-auth references for Confluence.
type ConfluenceConfig struct {
	Site     string `json:"site,omitempty"`     // e.g. acme.atlassian.net (fallback; the URL host wins)
	Email    string `json:"email,omitempty"`    // Basic-auth username
	TokenEnv string `json:"tokenEnv,omitempty"` // env var holding the API token
}

// GoogleConfig selects ONE auth mode: a service-account key file, or a refresh
// token. Both mint short-lived access tokens (see gauth.go).
type GoogleConfig struct {
	SAKeyFile string         `json:"saKeyFile,omitempty"` // path to a service-account JSON key
	Refresh   *GoogleRefresh `json:"refresh,omitempty"`   // OAuth refresh-token credentials
}

// GoogleRefresh names the env vars holding the OAuth client id/secret + refresh
// token — the values themselves stay out of the config file.
type GoogleRefresh struct {
	ClientEnv  string `json:"clientEnv,omitempty"`
	SecretEnv  string `json:"secretEnv,omitempty"`
	RefreshEnv string `json:"refreshEnv,omitempty"`
}

// LoadFetchConfig reads <projectDir>/.skillrunner/fetch.json. A missing file is
// not an error — it returns an empty config so flags/env alone can drive a run.
func LoadFetchConfig(projectDir string) (*FetchConfig, error) {
	path := filepath.Join(projectDir, ".skillrunner", "fetch.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &FetchConfig{}, nil
		}
		return nil, err
	}
	var cfg FetchConfig
	if err := decodeJSONC(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// firstNonEmpty returns the first non-empty argument (flag > env > config idiom).
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// expandHome expands a leading ~ in a path to the user's home dir.
func expandHome(p string) string {
	if p == "~" || len(p) >= 2 && p[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

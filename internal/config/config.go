// Package config resolves how DevEdu Code authenticates and where it talks to,
// and persists the user's settings locally so they only set up once.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultBaseURL is the DevEdu instance the CLI talks to unless overridden.
	DefaultBaseURL = "https://app.devedu.io"

	appDir   = "devedu"
	fileName = "config.json"
)

// Config holds everything the client needs to call the DevEdu API.
type Config struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

// Path returns the on-disk location of the stored config
// (e.g. ~/.config/devedu/config.json on Linux).
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appDir, fileName), nil
}

// Load resolves config in order of precedence: flag > DEVEDU_API_KEY_FILE >
// DEVEDU_API_KEY env > the stored config file > defaults. It does NOT prompt —
// callers handle a missing API key.
func Load(apiKeyFlag, baseURLFlag string) Config {
	stored, _ := loadFile()
	return Config{
		APIKey:  firstNonEmpty(apiKeyFlag, keyFromFile(), os.Getenv("DEVEDU_API_KEY"), stored.APIKey),
		BaseURL: firstNonEmpty(baseURLFlag, os.Getenv("DEVEDU_API_URL"), stored.BaseURL, DefaultBaseURL),
	}
}

// keyFromFile reads the API key from DEVEDU_API_KEY_FILE, if set. In a managed
// DevEdu container the platform mounts the user's key as a file (from a k8s
// Secret) so it's auto-loaded; reading it fresh means a regenerated key is
// picked up on the next run without re-entering anything.
func keyFromFile() string {
	path := os.Getenv("DEVEDU_API_KEY_FILE")
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func loadFile() (Config, error) {
	var c Config
	path, err := Path()
	if err != nil {
		return c, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err // a missing file is expected on first run
	}
	_ = json.Unmarshal(data, &c)
	return c, nil
}

// Save writes the config to disk with owner-only permissions (it holds a secret)
// and returns the path it was written to.
func (c Config) Save() (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

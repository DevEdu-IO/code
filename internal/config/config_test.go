package config

import (
	"os"
	"testing"
)

// isolateConfig points os.UserConfigDir at a temp dir so tests don't touch the
// real ~/.config, and clears the env vars that feed Load.
func isolateConfig(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEVEDU_API_KEY", "")
	t.Setenv("DEVEDU_API_URL", "")
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	isolateConfig(t)

	want := Config{APIKey: "dvedu_abc123", BaseURL: "https://school.devedu.io"}
	if _, err := want.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := Load("", "")
	if got.APIKey != want.APIKey {
		t.Errorf("APIKey = %q, want %q", got.APIKey, want.APIKey)
	}
	if got.BaseURL != want.BaseURL {
		t.Errorf("BaseURL = %q, want %q", got.BaseURL, want.BaseURL)
	}
}

func TestSavedFileIsOwnerOnly(t *testing.T) {
	isolateConfig(t)
	path, err := Config{APIKey: "secret"}.Save()
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %o, want 600 (config holds a secret)", perm)
	}
}

func TestLoadPrecedence(t *testing.T) {
	isolateConfig(t)
	// Stored file is the lowest-precedence source.
	if _, err := (Config{APIKey: "from_file", BaseURL: "https://file.example"}).Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// flag beats env beats file
	t.Setenv("DEVEDU_API_KEY", "from_env")
	got := Load("from_flag", "")
	if got.APIKey != "from_flag" {
		t.Errorf("flag should win: got %q", got.APIKey)
	}

	// env beats file when no flag
	got = Load("", "")
	if got.APIKey != "from_env" {
		t.Errorf("env should win over file: got %q", got.APIKey)
	}
}

func TestLoadDefaultsBaseURL(t *testing.T) {
	isolateConfig(t)
	if got := Load("", "").BaseURL; got != DefaultBaseURL {
		t.Errorf("BaseURL default = %q, want %q", got, DefaultBaseURL)
	}
}

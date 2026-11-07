package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// settings are the local, machine-only settings serve reads at startup
// from $XDG_CONFIG_HOME/code-central/settings.yaml (falling back to
// ~/.config/code-central/settings.yaml). A missing file means the
// defaults; an unknown key or bad value stops serve, naming the file.
type settings struct {
	// Notifications is on (the default) or off.
	Notifications string `yaml:"notifications"`
}

// settingsPath is where the local settings live, or "" if neither
// XDG_CONFIG_HOME nor HOME is set.
func settingsPath(getenv func(string) string) string {
	if d := getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "code-central", "settings.yaml")
	}
	if h := getenv("HOME"); h != "" {
		return filepath.Join(h, ".config", "code-central", "settings.yaml")
	}
	return ""
}

func loadSettings(getenv func(string) string) (settings, error) {
	s := settings{Notifications: "on"}
	path := settingsPath(getenv)
	if path == "" {
		return s, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("cannot read the settings: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil && !errors.Is(err, io.EOF) {
		return s, fmt.Errorf("%s: %v", path, err)
	}
	if s.Notifications != "on" && s.Notifications != "off" {
		return s, fmt.Errorf("%s: field notifications: must be on or off, not %q", path, s.Notifications)
	}
	return s, nil
}

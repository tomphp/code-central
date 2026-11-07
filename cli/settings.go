package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Settings are the human's local settings for the service, in
// settings.yaml under the user's config folder. They never live in a
// repository, so changing one never changes a committed file. A missing
// file means every default.
//
// Add new settings here as fields, each with its default applied in
// loadSettings.
type Settings struct {
	// Port replaces 7733 for a real clash; point agents at it with
	// CODE_CENTRAL_URL.
	Port int `yaml:"port"`
}

// settingsPath is $XDG_CONFIG_HOME/code-central/settings.yaml, else
// ~/.config/code-central/settings.yaml; "" when neither is known.
func settingsPath(getenv func(string) string) string {
	if d := getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "code-central", "settings.yaml")
	}
	if h := getenv("HOME"); h != "" {
		return filepath.Join(h, ".config", "code-central", "settings.yaml")
	}
	return ""
}

// loadSettings reads the settings strictly: an unknown key or a bad value
// stops serve and names the file.
func loadSettings(getenv func(string) string) (Settings, error) {
	s := Settings{Port: Port}
	path := settingsPath(getenv)
	if path == "" {
		return s, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("cannot read %s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil && !errors.Is(err, io.EOF) {
		return s, fmt.Errorf("%s: %w", path, err)
	}
	if s.Port < 1 || s.Port > 65535 {
		return s, fmt.Errorf("%s: port must be between 1 and 65535, not %d", path, s.Port)
	}
	return s, nil
}

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/tomoram/code-central/home"
	"gopkg.in/yaml.v3"
)

// settings are the local, machine-only settings serve reads at startup
// from $XDG_CONFIG_HOME/code-central/settings.yaml (falling back to
// ~/.config/code-central/settings.yaml). They never live in a repository,
// so changing one never changes a committed file. A missing file means
// the defaults; an unknown key or bad value stops serve, naming the file.
// Add new settings as fields here, defaulted in loadSettings.
type settings struct {
	// Port replaces 7733 for a real clash; point agents at it with
	// CODE_CENTRAL_URL.
	Port int `yaml:"port"`
	// Notifications is on (the default) or off.
	Notifications string `yaml:"notifications"`
}

func loadSettings(h home.Lookup) (settings, error) {
	s := settings{Port: Port, Notifications: "on"}
	// With no home at all there are no settings, only the defaults.
	path, err := h.Config("code-central", "settings.yaml")
	if err != nil {
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
	if s.Port < 1 || s.Port > 65535 {
		return s, fmt.Errorf("%s: field port: must be between 1 and 65535, not %d", path, s.Port)
	}
	if s.Notifications != "on" && s.Notifications != "off" {
		return s, fmt.Errorf("%s: field notifications: must be on or off, not %q", path, s.Notifications)
	}
	return s, nil
}

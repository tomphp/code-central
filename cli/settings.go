package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/tomoram/code-central/app"
	"github.com/tomoram/code-central/home"
	"gopkg.in/yaml.v3"
)

// settings are the local, machine-only settings serve reads at startup
// from $XDG_CONFIG_HOME/code-central/settings.yaml (falling back to
// ~/.config/code-central/settings.yaml, where ~ is HOME or else the
// profile folder: ADR 0056). They never live in a repository,
// so changing one never changes a committed file. A missing file means
// the defaults; an unknown key or bad value stops serve, naming the file.
// Add new settings as fields here, defaulted in loadSettings.
type settings struct {
	// Port replaces 7733 for a real clash; point agents at it with
	// CODE_CENTRAL_URL.
	Port int `yaml:"port"`
	// Notifications is on (the default) or off.
	Notifications string `yaml:"notifications"`
	// Store names the Store Kind and where it is (ADR 0067); nil, the
	// default, is a File Store in the data folder. The --store flag and
	// CODE_CENTRAL_STORE override it with a File Store at their folder.
	Store *storeSettings `yaml:"store"`
}

// storeSettings is the store: block.
type storeSettings struct {
	// Kind is files or sqlite.
	Kind string `yaml:"kind"`
	// Path is the File Store's folder or the SQLite database file; empty
	// means the data folder, or code-central.db in it.
	Path string `yaml:"path"`
	// URLEnv is the environment variable a postgres store reads its URL
	// from, which no Kind this build opens has.
	URLEnv string `yaml:"url_env"`
}

// storeKinds are the Kinds the store: block accepts; serve.go maps them
// to the store's.
var storeKinds = []string{"files", "sqlite"}

func loadSettings(h home.Lookup) (settings, error) {
	s := settings{Port: app.Port, Notifications: "on"}
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
	if st := s.Store; st != nil {
		if !slices.Contains(storeKinds, st.Kind) {
			return s, fmt.Errorf("%s: field store.kind: must be files or sqlite, not %q", path, st.Kind)
		}
		if st.URLEnv != "" {
			return s, fmt.Errorf("%s: field store.url_env: only a postgres store reads its URL from the environment; a %s store has a path", path, st.Kind)
		}
	}
	return s, nil
}

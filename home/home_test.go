package home_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/tomoram/code-central/home"
)

func lookup(env map[string]string, profile string) home.Lookup {
	return home.Lookup{
		Getenv: func(k string) string { return env[k] },
		UserHomeDir: func() (string, error) {
			if profile == "" {
				return "", errors.New("%USERPROFILE% is not defined")
			}
			return profile, nil
		},
	}
}

func TestEachFolderIsItsXDGVariableThenHOMEThenTheProfileFolder(t *testing.T) {
	p := filepath.Join
	cases := []struct {
		name    string
		folder  func(home.Lookup, ...string) (string, error)
		env     map[string]string
		profile string
		want    string
	}{
		{"data from XDG", home.Lookup.Data, map[string]string{"XDG_DATA_HOME": "/xdg", "HOME": "/home/me"}, "/profile", p("/xdg", "a")},
		{"data from HOME", home.Lookup.Data, map[string]string{"HOME": "/home/me"}, "/profile", p("/home/me", ".local", "share", "a")},
		{"data from the profile", home.Lookup.Data, nil, "/profile", p("/profile", ".local", "share", "a")},
		{"config from XDG", home.Lookup.Config, map[string]string{"XDG_CONFIG_HOME": "/xdg", "HOME": "/home/me"}, "/profile", p("/xdg", "a")},
		{"config from HOME", home.Lookup.Config, map[string]string{"HOME": "/home/me"}, "/profile", p("/home/me", ".config", "a")},
		{"config from the profile", home.Lookup.Config, nil, "/profile", p("/profile", ".config", "a")},
		// XDG_STATE_HOME isn't read: the state files stay where they always were.
		{"state from HOME", home.Lookup.State, map[string]string{"XDG_STATE_HOME": "/xdg", "HOME": "/home/me"}, "/profile", p("/home/me", ".local", "state", "a")},
		{"state from the profile", home.Lookup.State, nil, "/profile", p("/profile", ".local", "state", "a")},
		{"home from HOME", func(l home.Lookup, elem ...string) (string, error) { return l.Home(elem...) }, map[string]string{"HOME": "/home/me"}, "/profile", p("/home/me", "a")},
		{"home from the profile", func(l home.Lookup, elem ...string) (string, error) { return l.Home(elem...) }, nil, "/profile", p("/profile", "a")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.folder(lookup(c.env, c.profile), "a")
			if err != nil || got != c.want {
				t.Fatalf("got %q, %v; want %q", got, err, c.want)
			}
		})
	}
}

func TestNoHomeAtAllNamesEveryOption(t *testing.T) {
	_, err := lookup(nil, "").Data()
	var nf *home.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err %v, want a NotFoundError", err)
	}
	want := "set XDG_DATA_HOME or HOME, or give this account a profile folder (%USERPROFILE% on Windows)"
	if err.Error() != want {
		t.Fatalf("err %q, want %q", err, want)
	}
	nf.Vars = append([]string{"CODE_CENTRAL_STORE"}, nf.Vars...)
	if want := "set CODE_CENTRAL_STORE, XDG_DATA_HOME or HOME, or give this account a profile folder (%USERPROFILE% on Windows)"; err.Error() != want {
		t.Fatalf("err %q, want %q", err, want)
	}
	if _, err := (home.Lookup{Getenv: func(string) string { return "" }}).Home(); err == nil || err.Error() != "set HOME, or give this account a profile folder (%USERPROFILE% on Windows)" {
		t.Fatalf("no UserHomeDir: err %v", err)
	}
}

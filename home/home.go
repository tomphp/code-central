// Package home finds the folders Code Central's files live in. They use
// the XDG layout on every OS (ADR 0010): each folder is its XDG_*
// variable, else under HOME, else under the account's profile folder
// (%USERPROFILE% on Windows, which usually has no HOME).
package home

import (
	"errors"
	"path/filepath"
	"strings"
)

// Lookup is where the folders come from. Both parts are injected, so
// tests can drive it on any OS.
type Lookup struct {
	Getenv func(string) string
	// UserHomeDir is the profile folder, used when HOME isn't set; main
	// passes os.UserHomeDir. nil means there is none, so a test never
	// finds the real one.
	UserHomeDir func() (string, error)
}

// NotFoundError is a folder with no home to put it under. Vars are the
// variables that would have named it, before HOME; a caller with an
// override of its own puts that first.
type NotFoundError struct{ Vars []string }

func (e *NotFoundError) Error() string {
	vars := append(append([]string{}, e.Vars...), "HOME")
	list := vars[len(vars)-1]
	if len(vars) > 1 {
		list = strings.Join(vars[:len(vars)-1], ", ") + " or " + list
	}
	return "set " + list + ", or give this account a profile folder (%USERPROFILE% on Windows)"
}

// Home is elem under the home folder: HOME, else the profile folder.
func (l Lookup) Home(elem ...string) (string, error) {
	return l.under("", nil, elem)
}

// Data is elem under $XDG_DATA_HOME, else ~/.local/share.
func (l Lookup) Data(elem ...string) (string, error) {
	return l.under("XDG_DATA_HOME", []string{".local", "share"}, elem)
}

// Config is elem under $XDG_CONFIG_HOME, else ~/.config.
func (l Lookup) Config(elem ...string) (string, error) {
	return l.under("XDG_CONFIG_HOME", []string{".config"}, elem)
}

// State is elem under ~/.local/state. XDG_STATE_HOME isn't read, so the
// state files stay where they always were on macOS and Linux.
func (l Lookup) State(elem ...string) (string, error) {
	return l.under("", []string{".local", "state"}, elem)
}

// StartMenu is elem under the Start menu's Programs folder on Windows:
// under %APPDATA%, else the profile folder's AppData\Roaming. HOME isn't
// read, because Windows keeps the Start menu in the profile whatever HOME
// says.
func (l Lookup) StartMenu(elem ...string) (string, error) {
	d := l.Getenv("APPDATA")
	if d == "" && l.UserHomeDir != nil {
		if p, err := l.UserHomeDir(); err == nil && p != "" {
			d = filepath.Join(p, "AppData", "Roaming")
		}
	}
	if d == "" {
		return "", errors.New("cannot find the Start menu: set APPDATA, or give this account a profile folder (%USERPROFILE%)")
	}
	return filepath.Join(append([]string{d, "Microsoft", "Windows", "Start Menu", "Programs"}, elem...)...), nil
}

// under is elem under $xdg, or else under the home folder's fallback.
func (l Lookup) under(xdg string, fallback, elem []string) (string, error) {
	if xdg != "" {
		if d := l.Getenv(xdg); d != "" {
			return filepath.Join(append([]string{d}, elem...)...), nil
		}
	}
	h := l.Getenv("HOME")
	if h == "" && l.UserHomeDir != nil {
		if d, err := l.UserHomeDir(); err == nil {
			h = d
		}
	}
	if h == "" {
		nf := &NotFoundError{}
		if xdg != "" {
			nf.Vars = []string{xdg}
		}
		return "", nf
	}
	return filepath.Join(append(append([]string{h}, fallback...), elem...)...), nil
}

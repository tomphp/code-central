package claude

import "errors"

// Locate finds claude with lookPath, which finds a program on PATH, and
// refuses one that can't take the Factory's prompts.
func Locate(lookPath func(name string) (string, bool)) (string, error) {
	path, ok := lookPath("claude")
	if !ok {
		return "", errors.New("claude isn't on PATH")
	}
	if err := refuseShim(path); err != nil {
		return "", err
	}
	return path, nil
}

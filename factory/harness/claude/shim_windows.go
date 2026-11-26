package claude

import (
	"fmt"
	"path/filepath"
	"strings"
)

// refuseShim refuses a claude that isn't the native claude.exe, such as
// the npm shim claude.cmd: Go runs a .cmd or .bat through cmd.exe, which
// cuts arguments at a newline and expands %...%, so a multi-line prompt
// wouldn't arrive intact.
func refuseShim(path string) error {
	if strings.EqualFold(filepath.Ext(path), ".exe") {
		return nil
	}
	return fmt.Errorf("claude on PATH is %s, not the native claude.exe: %s, like the npm shim claude.cmd, can't carry a multi-line prompt, because Windows runs it through cmd.exe, which cuts arguments at a newline. "+
		"Install the native claude.exe with `irm https://claude.ai/install.ps1 | iex` in PowerShell, put it before the npm folder on PATH, and start the Factory again", path, filepath.Base(path))
}

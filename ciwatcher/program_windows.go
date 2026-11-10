// How a program is found on Windows.

package ciwatcher

import "os"

// exeSuffix ends the name of a program on PATH: gh's installer puts
// gh.exe there.
const exeSuffix = ".exe"

// executable reports whether fi may be run: any file may, on Windows.
func executable(os.FileInfo) bool { return true }

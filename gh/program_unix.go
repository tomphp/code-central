//go:build unix

// How a program is found on Unix.

package gh

import "os"

// exeSuffix ends the name of a program on PATH.
const exeSuffix = ""

// executable reports whether fi may be run.
func executable(fi os.FileInfo) bool { return fi.Mode()&0o111 != 0 }

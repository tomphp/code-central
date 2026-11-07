//go:build !webdist

package web

import "embed"

//go:embed placeholder
var files embed.FS

const root = "placeholder"

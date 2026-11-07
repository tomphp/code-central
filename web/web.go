// Package web serves the embedded UI.
//
// By default it embeds the committed placeholder page, so `go build` and
// Go tests need no Node. `make build` runs Vite into web/dist and builds
// with `-tags webdist` to embed the real UI instead.
package web

import (
	"io/fs"
	"net/http"
)

// Handler serves the UI.
func Handler() http.Handler {
	sub, err := fs.Sub(files, root)
	if err != nil {
		panic(err)
	}
	return http.FileServerFS(sub)
}

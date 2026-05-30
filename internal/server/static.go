package server

import (
	"embed"
	"io/fs"
	"net/http"
)

// staticFS holds the embedded web assets served at "/".
//
// This is the Slice 5 mount point: today it carries only a placeholder
// index.html, but the web bundle drops straight into internal/server/static
// and is picked up here with no handler changes.
//
//go:embed static
var staticFS embed.FS

// staticHandler serves the embedded web assets rooted at the "static" subtree.
func staticHandler() (http.Handler, error) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, err
	}
	return http.FileServer(http.FS(sub)), nil
}

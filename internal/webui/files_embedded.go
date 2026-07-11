//go:build webui

package webui

import (
	"embed"
	"io/fs"
)

//go:embed dist
var embedded embed.FS

// Files returns the production Vite bundle embedded by make production.
func Files() (fs.FS, bool) {
	files, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err)
	}
	return files, true
}

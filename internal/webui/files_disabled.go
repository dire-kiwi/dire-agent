//go:build !webui

package webui

import "io/fs"

// Files reports that a normal development build has no embedded Web UI.
func Files() (fs.FS, bool) { return nil, false }

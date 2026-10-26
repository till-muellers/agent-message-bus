package web

import (
	"embed"
	"fmt"
	"io/fs"
)

// bundle holds the GUI build output. Only dist/.gitkeep is tracked; the GUI build
// writes dist/index.html and dist/assets/*. "all:" embeds the dotfile so the pattern
// matches even when the GUI was not built.
//
//go:embed all:dist
var bundle embed.FS

// Dist returns the embedded GUI bundle rooted at dist/.
func Dist() (fs.FS, error) {
	sub, err := fs.Sub(bundle, "dist")
	if err != nil {
		return nil, fmt.Errorf("web bundle: %w", err)
	}
	return sub, nil
}

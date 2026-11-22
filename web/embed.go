// Package web embeds the static timeline UI into the binary.
package web

import "embed"

// Static holds web/static/* (index.html, styles.css, *.js).
//
//go:embed static
var Static embed.FS

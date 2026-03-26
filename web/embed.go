package web

import "embed"

// DistFS contains the built React SPA assets.
// Build with: cd web && npm run build
//
//go:embed all:dist
var DistFS embed.FS

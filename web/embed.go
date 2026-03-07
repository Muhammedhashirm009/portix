package web

import "embed"

// FS embeds all web assets (static files and templates).
// Paths within the FS start from "static/" and "templates/"
// (relative to this file's directory).
//
//go:embed all:static all:templates
var FS embed.FS

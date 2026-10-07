// Package assets embeds what taproot carries with it, so the binary is
// self-contained: the page taproot serve hands out (one html file, its
// stylesheet and its script), and the scenes that ship with taproot.
package assets

import "embed"

// PageHTML is the page. {{version}} in it is replaced with taproot's version
// as it is served.
//
//go:embed page.html
var PageHTML string

// PageCSS is the page's stylesheet.
//
//go:embed page.css
var PageCSS string

// PageJS is the page's script: it draws the panels and talks to the API.
//
//go:embed page.js
var PageJS string

// Scenes are the scenes taproot ships with, one file each in the
// controller's own format, under scenes/. They are in every library as
// "built in", so a scene lost from a controller is never further away than
// the binary; a scene of the same name kept in the library's own directory
// stands in for the built-in one.
//
//go:embed scenes/*.json
var Scenes embed.FS

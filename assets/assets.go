// Package assets embeds the page taproot serve hands out, so the binary is
// self-contained: one html file, its stylesheet and its script.
package assets

import _ "embed" // the page's three files

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

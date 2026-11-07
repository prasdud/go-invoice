// Package templates holds the compile-time invoice themes. Each theme is one
// Go file in this package that embeds its HTML asset and registers itself in
// init; themes are parsed once at process start and never touched per request.
package templates

import "html/template"

// Theme is a parsed, immutable invoice layout.
type Theme struct {
	Name string
	HTML *template.Template
}

var registry = map[string]Theme{}

func register(name string, html *template.Template) {
	registry[name] = Theme{Name: name, HTML: html}
}

// Get returns the theme registered under name.
func Get(name string) (Theme, bool) {
	t, ok := registry[name]
	return t, ok
}

// Default returns the built-in default theme.
func Default() Theme {
	t, ok := registry["default"]
	if !ok {
		panic("templates: default theme not registered")
	}
	return t
}

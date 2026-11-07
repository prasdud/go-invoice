package templates

import (
	_ "embed"
	"html/template"
)

//go:embed default.html
var defaultHTML string

func init() {
	register("default", template.Must(template.New("default").Parse(defaultHTML)))
}

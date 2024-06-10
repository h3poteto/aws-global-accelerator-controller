package templates

import _ "embed"

//go:embed issuer.yaml.tmpl
var issuerTmpl string

//go:embed certificate.yaml.tmpl
var certificateTmpl string

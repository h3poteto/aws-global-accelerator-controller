package templates

import _ "embed"

//go:embed issuer.yaml.tmpl
var issuerTmpl string

//go:embed certificate.yaml.tmpl
var certificateTmpl string

//go:embed webhook.yaml.tmpl
var webhookTmpl string

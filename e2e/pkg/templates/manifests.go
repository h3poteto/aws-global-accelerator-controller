package templates

import (
	"bytes"
	"text/template"
)

func Issuer(name, ns string) (*bytes.Buffer, error) {
	params := map[string]interface{}{
		"IssuerName": name,
		"Namespace":  ns,
	}

	tpl, err := template.New("issuer").Parse(issuerTmpl)
	if err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	if err := tpl.Execute(buf, params); err != nil {
		return nil, err
	}
	return buf, nil
}

func Certificate(name, ns, issuerName, serviceName, secretName string) (*bytes.Buffer, error) {
	params := map[string]interface{}{
		"CertificateName": name,
		"Namespace":       ns,
		"IssuerName":      issuerName,
		"ServiceName":     serviceName,
		"CertSecretName":  secretName,
	}

	tpl, err := template.New("certificate").Parse(certificateTmpl)
	if err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	if err := tpl.Execute(buf, params); err != nil {
		return nil, err
	}
	return buf, nil
}

func WebhookConfiguration(certificateNamespace, certificateName, serviceNamespace, serviceName, serviceEndpoint string) (*bytes.Buffer, error) {
	params := map[string]interface{}{
		"CertificateNamespace": certificateNamespace,
		"CertificateName":      certificateName,
		"ServiceName":          serviceName,
		"ServiceNamespace":     serviceNamespace,
		"ServiceEndpoint":      serviceEndpoint,
	}
	tpl, err := template.New("webhook").Parse(webhookTmpl)
	if err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	if err := tpl.Execute(buf, params); err != nil {
		return nil, err
	}
	return buf, nil
}

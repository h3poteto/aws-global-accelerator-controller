package util

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/h3poteto/aws-global-accelerator-controller/e2e/pkg/templates"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	serializeryaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

var (
	issuerName           = "e2e"
	certificateName      = "e2e"
	certificateNamespace = "default"
)

func ApplyCRD(ctx context.Context, cfg *rest.Config) error {
	p, err := os.Getwd()
	if err != nil {
		return err
	}
	path := filepath.Join(p, "../config/crd/operator.h3poteto.dev_endpointgroupbindings.yaml")

	buf, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return apply(ctx, cfg, buf)
}

func ApplyWebhook(ctx context.Context, cfg *rest.Config, serviceNS, serviceName, serviceEndpoint string) error {
	webhookconfiguration, err := templates.WebhookConfiguration(certificateNamespace, certificateName, serviceNS, serviceName, serviceEndpoint)
	if err != nil {
		return err
	}
	return apply(ctx, cfg, webhookconfiguration.Bytes())
}

func ApplyIssuer(ctx context.Context, cfg *rest.Config) error {
	issuer, err := templates.Issuer(issuerName, certificateNamespace)
	if err != nil {
		return err
	}
	return apply(ctx, cfg, issuer.Bytes())
}

func ApplyCertificate(ctx context.Context, cfg *rest.Config, serivceNS, service, secretNS, secret string) error {
	certificate, err := templates.Certificate(certificateName, certificateNamespace, issuerName, service, secret)
	if err != nil {
		return err
	}
	return apply(ctx, cfg, certificate.Bytes())
}

func apply(ctx context.Context, cfg *rest.Config, data []byte) error {
	return restAction(ctx, cfg, data, func(ctx context.Context, dr dynamic.ResourceInterface, obj *unstructured.Unstructured, data []byte) error {
		_, err := dr.Patch(ctx, obj.GetName(), types.ApplyPatchType, data, v1.PatchOptions{
			FieldManager: "e2e",
		})
		return err
	})
}

type actionFunc func(ctx context.Context, dr dynamic.ResourceInterface, obj *unstructured.Unstructured, data []byte) error

func restAction(ctx context.Context, cfg *rest.Config, data []byte, action actionFunc) error {
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return err
	}

	multidocReader := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))

	for {
		buf, err := multidocReader.Read()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var typeMeta runtime.TypeMeta
		if err := yaml.Unmarshal(buf, &typeMeta); err != nil {
			continue
		}
		if typeMeta.Kind == "" {
			continue
		}

		obj := &unstructured.Unstructured{}
		_, gvk, err := serializeryaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme).Decode(buf, nil, obj)
		if err != nil {
			return err
		}

		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return err
		}

		var dr dynamic.ResourceInterface
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			dr = dyn.Resource(mapping.Resource).Namespace(obj.GetNamespace())
		} else {
			dr = dyn.Resource(mapping.Resource)
		}

		data, err := json.Marshal(obj)
		if err != nil {
			return err
		}

		if err := action(ctx, dr, obj, data); err != nil {
			return err
		}
	}
}

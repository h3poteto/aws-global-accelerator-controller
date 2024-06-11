package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/fixture"
	admissionv1 "k8s.io/api/admission/v1"
	authenicationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
)

func TestHealthz(t *testing.T) {
	reqBody := bytes.NewBufferString("")
	req := httptest.NewRequest("GET", "/healthz", reqBody)
	response := httptest.NewRecorder()

	Healthz(response, req)

	if response.Code != 200 {
		t.Errorf("Expected status code 200, but got %d", response.Code)
	}
}

func TestValidateEndpointGroupBinding_updateWeight(t *testing.T) {
	old := fixture.EndpointGroupBinding(false, "example", nil, "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcd1234")
	new := fixture.EndpointGroupBinding(false, "example", aws.Int32(100), "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcd1234")
	rawNew, err := json.Marshal(new)
	if err != nil {
		t.Errorf("Failed to marshal json: %v", err)
	}
	rawObjNew := runtime.RawExtension{
		Raw:    rawNew,
		Object: runtime.Object(&new),
	}
	rawOld, err := json.Marshal(old)
	if err != nil {
		t.Errorf("Failed to marshal json: %v", err)
	}
	rawObjOld := runtime.RawExtension{
		Raw:    rawOld,
		Object: runtime.Object(&old),
	}

	review := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
		Request: &admissionv1.AdmissionRequest{
			UID: uuid.NewUUID(),
			Kind: metav1.GroupVersionKind{
				Group:   "operator.h3poteto.dev",
				Version: "v1alpha1",
				Kind:    "EndpointGroupBinding",
			},
			Resource: metav1.GroupVersionResource{
				Group:    "operator.h3poteto.dev",
				Version:  "v1alpha1",
				Resource: "endpointgroupbindings",
			},
			RequestKind: &metav1.GroupVersionKind{
				Group:   "operator.h3poteto.dev",
				Version: "v1alpha1",
				Kind:    "EndpointGroupBinding",
			},
			RequestResource: &metav1.GroupVersionResource{
				Group:    "operator.h3poteto.dev",
				Version:  "v1alpha1",
				Resource: "endpointgroupbindings",
			},
			Name:      "example",
			Namespace: "kube-system",
			Operation: admissionv1.Update,
			UserInfo: authenicationv1.UserInfo{
				Username: "h3poteto",
				UID:      string(uuid.NewUUID()),
				Groups:   []string{},
				Extra: map[string]authenicationv1.ExtraValue{
					"": {},
				},
			},
			Object:    rawObjNew,
			OldObject: rawObjOld,
			DryRun:    aws.Bool(false),
		},
	}
	jbody, err := json.Marshal(review)
	if err != nil {
		t.Errorf("Failed to marshal json: %v", err)
		return
	}
	reqBody := bytes.NewBuffer(jbody)
	req := httptest.NewRequest("POST", "/validate-endpointgroupbinding", reqBody)
	req.Header.Set("Content-Type", "application/json")

	response := httptest.NewRecorder()
	ValidateEndpointGroupBinding(response, req)

	if response.Code != 200 {
		t.Errorf("Expected status code 200, but got %d", response.Code)
		return
	}
	body, err := parseResponse(response.Body)
	if err != nil {
		t.Errorf("Failed to parse response: %v", err)
	}
	if !body.Response.Allowed {
		t.Errorf("Expected allowed, but got not allowed")
	}
	if body.Response.Result.Code != http.StatusOK {
		t.Errorf("Expected status code 200, but got %d", body.Response.Result.Code)
	}
}

func TestValidateEndpointGroupBinding_updateArn(t *testing.T) {
	old := fixture.EndpointGroupBinding(false, "example", nil, "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcd1234")
	new := fixture.EndpointGroupBinding(false, "example", aws.Int32(100), "arn:aws:globalaccelerator::123456789012:accelerator/5678efgh-efgh-5678-efgh-5678efgh5678")
	rawNew, err := json.Marshal(new)
	if err != nil {
		t.Errorf("Failed to marshal json: %v", err)
	}
	rawObjNew := runtime.RawExtension{
		Raw:    rawNew,
		Object: runtime.Object(&new),
	}
	rawOld, err := json.Marshal(old)
	if err != nil {
		t.Errorf("Failed to marshal json: %v", err)
	}
	rawObjOld := runtime.RawExtension{
		Raw:    rawOld,
		Object: runtime.Object(&old),
	}

	review := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
		Request: &admissionv1.AdmissionRequest{
			UID: uuid.NewUUID(),
			Kind: metav1.GroupVersionKind{
				Group:   "operator.h3poteto.dev",
				Version: "v1alpha1",
				Kind:    "EndpointGroupBinding",
			},
			Resource: metav1.GroupVersionResource{
				Group:    "operator.h3poteto.dev",
				Version:  "v1alpha1",
				Resource: "endpointgroupbindings",
			},
			RequestKind: &metav1.GroupVersionKind{
				Group:   "operator.h3poteto.dev",
				Version: "v1alpha1",
				Kind:    "EndpointGroupBinding",
			},
			RequestResource: &metav1.GroupVersionResource{
				Group:    "operator.h3poteto.dev",
				Version:  "v1alpha1",
				Resource: "endpointgroupbindings",
			},
			Name:      "example",
			Namespace: "kube-system",
			Operation: admissionv1.Update,
			UserInfo: authenicationv1.UserInfo{
				Username: "h3poteto",
				UID:      string(uuid.NewUUID()),
				Groups:   []string{},
				Extra: map[string]authenicationv1.ExtraValue{
					"": {},
				},
			},
			Object:    rawObjNew,
			OldObject: rawObjOld,
			DryRun:    aws.Bool(false),
		},
	}
	jbody, err := json.Marshal(review)
	if err != nil {
		t.Errorf("Failed to marshal json: %v", err)
		return
	}
	reqBody := bytes.NewBuffer(jbody)
	req := httptest.NewRequest("POST", "/validate-endpointgroupbinding", reqBody)
	req.Header.Set("Content-Type", "application/json")

	response := httptest.NewRecorder()
	ValidateEndpointGroupBinding(response, req)

	if response.Code != 200 {
		t.Errorf("Expected status code 200, but got %d", response.Code)
	}
	body, err := parseResponse(response.Body)
	if err != nil {
		t.Errorf("Failed to parse response: %v", err)
	}
	if body.Response.Allowed {
		t.Errorf("Expected not allowed, but got allowed")
	}
	if body.Response.Result.Code != http.StatusForbidden {
		t.Errorf("Expected status code 403, but got %d", body.Response.Result.Code)
	}
}

func parseResponse(body *bytes.Buffer) (*admissionv1.AdmissionReview, error) {
	var review admissionv1.AdmissionReview
	if err := json.Unmarshal(body.Bytes(), &review); err != nil {
		return nil, err
	}
	return &review, nil
}

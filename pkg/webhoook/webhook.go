package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/h3poteto/aws-global-accelerator-controller/pkg/webhoook/endpointgroupbinding"
	admissionv1 "k8s.io/api/admission/v1"
	klog "k8s.io/klog/v2"
)

func Server(tlsCertFile, tlsKeyFile string) {
	http.HandleFunc("/healthz", Healthz)
	http.HandleFunc("/validate-endpointgroupbinding", ValidateEndpointGroupBinding)

	klog.Infof("Listening on :443")
	err := http.ListenAndServeTLS(":443", tlsCertFile, tlsKeyFile, nil)
	if err != nil {
		klog.Fatalf("Failed to start server: %v", err)
	}
}

func Healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func ValidateEndpointGroupBinding(w http.ResponseWriter, r *http.Request) {

	in, err := parseRequest(*r)
	if err != nil {
		klog.Error(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	response := endpointgroupbinding.Validate(in)
	out, err := json.Marshal(response)
	if err != nil {
		klog.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(out)
}

func parseRequest(r http.Request) (*admissionv1.AdmissionReview, error) {
	if r.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("invalid Content-Type")
	}

	bodybuf := new(bytes.Buffer)
	bodybuf.ReadFrom(r.Body)
	body := bodybuf.Bytes()

	if len(body) == 0 {
		return nil, fmt.Errorf("empty body")
	}

	var review admissionv1.AdmissionReview

	if err := json.Unmarshal(body, &review); err != nil {
		return nil, fmt.Errorf("failed to unmarshal body: %v", err)
	}

	if review.Request == nil {
		return nil, fmt.Errorf("empty request")
	}

	return &review, nil
}

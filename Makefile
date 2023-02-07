.PHONY: build run clean manifests controller-gen push

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

CRD_OPTIONS ?= "crd"
CONTROLLER_TOOLS_TAG=v0.10.0
BRANCH := $(shell git branch --show-current)

build: manifests
	go build -a -tags netgo -installsuffix netgo -ldflags \
" \
  -extldflags '-static' \
  -X github.com/h3poteto/aws-global-accelerator-controller/cmd.version=$(shell git describe --tag --abbrev=0) \
  -X github.com/h3poteto/aws-global-accelerator-controller/cmd.revision=$(shell git rev-list -1 HEAD) \
  -X github.com/h3poteto/aws-global-accelerator-controller/cmd.build=$(shell git describe --tags) \
"

run: manifests
	go run ./main.go controller --kubeconfig=${KUBECONFIG}


clean:
	rm -f $(GOBIN)/controller-gen

manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=global-accelerator-manager-role paths=./...


controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go install sigs.k8s.io/controller-tools/cmd/controller-gen@${CONTROLLER_TOOLS_TAG} ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

push:
	docker build -f Dockerfile -t ghcr.io/h3poteto/aws-global-accelerator-controller:$(BRANCH) .
	docker push ghcr.io/h3poteto/aws-global-accelerator-controller:$(BRANCH)

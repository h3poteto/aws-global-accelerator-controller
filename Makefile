.PHONY: build run clean manifests code-generator controller-gen

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

CRD_OPTIONS ?= "crd"
CODE_GENERATOR=${GOPATH}/src/k8s.io/code-generator
CODE_GENERATOR_TAG=v0.23.4
CONTROLLER_TOOLS_TAG=v0.8.0
BRANCH := $(shell git branch --show-current)

build: manifests
	go build -a -tags netgo -installsuffix netgo --ldflags '-extldflags "-static"'

run: manifests
	go run ./main.go controller --kubeconfig=${KUBECONFIG}


clean:
	rm -f $(GOBIN)/controller-gen
	rm -rf $(CODE_GENERATOR)

manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=global-accelerator-manager-role paths=./...

code-generator:
ifeq (, $(wildcard ${CODE_GENERATOR}))
	git clone https://github.com/kubernetes/code-generator.git ${CODE_GENERATOR} -b ${CODE_GENERATOR_TAG} --depth 1
endif

controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@${CONTROLLER_TOOLS_TAG} ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

gpush:
	docker build -f Dockerfile -t ghcr.io/h3poteto/aws-global-accelerator-controller:$(BRANCH) .
	docker push ghcr.io/h3poteto/aws-global-accelerator-controller:$(BRANCH)

.PHONY: build run clean manifests controller-gen push

# Get the currently used golang install path
# Use ~/.local/bin for local development if it exists in PATH, otherwise use go bin
ifneq (,$(and $(findstring $(HOME)/.local/bin,$(PATH)),$(wildcard $(HOME)/.local/bin)))
GOBIN=$(HOME)/.local/bin
else
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif
endif

CRD_OPTIONS ?= crd
CODE_GENERATOR=${GOPATH}/src/k8s.io/code-generator
CODE_GENERATOR_TAG=v0.33.3
CONTROLLER_TOOLS_TAG=v0.18.0
BRANCH := $(shell git branch --show-current)

TARGETOS ?= linux
TARGETARCH ?= amd64

build: codegen manifests
	GOOS=$(TARGETOS) GOARCH=$(TARGETARCH) CGO_ENABLED="0" \
	go build -a -tags netgo -installsuffix netgo -ldflags \
" \
  -extldflags '-static' \
  -X github.com/h3poteto/aws-global-accelerator-controller/cmd.version=$(shell git describe --tag --abbrev=0) \
  -X github.com/h3poteto/aws-global-accelerator-controller/cmd.revision=$(shell git rev-list -1 HEAD) \
  -X github.com/h3poteto/aws-global-accelerator-controller/cmd.build=$(shell git describe --tags) \
"

run: codegen manifests
	go run ./main.go controller --kubeconfig=${KUBECONFIG}

install: manifests
	kubectl apply -f ./config/crd

clean:
	rm -f $(GOBIN)/controller-gen
	rm -rf $(CODE_GENERATOR)

codegen: code-generator
	@if command -v asdf >/dev/null 2>&1; then \
		GOBIN=$$(asdf exec go env GOBIN) CODE_GENERATOR=${CODE_GENERATOR} hack/update-codegen.sh; \
	else \
		CODE_GENERATOR=${CODE_GENERATOR} hack/update-codegen.sh; \
	fi

code-generator:
ifeq (, $(wildcard ${CODE_GENERATOR}))
	git clone https://github.com/kubernetes/code-generator.git ${CODE_GENERATOR} -b ${CODE_GENERATOR_TAG} --depth 1
endif


manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=global-accelerator-manager-role webhook paths=./... output:crd:artifacts:config=./config/crd/ output:webhook:artifacts:config=./config/webhook/


controller-gen:
ifeq (, $(shell which controller-gen))
	@echo "controller-gen not found, downloading..."
	curl -L -o controller-gen https://github.com/kubernetes-sigs/controller-tools/releases/download/${CONTROLLER_TOOLS_TAG}/controller-gen-linux-amd64
	chmod +x controller-gen
	mv controller-gen $(GOBIN)/controller-gen
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

push:
	docker build -f Dockerfile -t ghcr.io/h3poteto/aws-global-accelerator-controller:$(BRANCH) .
	docker push ghcr.io/h3poteto/aws-global-accelerator-controller:$(BRANCH)

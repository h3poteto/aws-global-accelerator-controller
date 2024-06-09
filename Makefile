.PHONY: build run clean manifests controller-gen push

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

CRD_OPTIONS ?= crd
CODE_GENERATOR=${GOPATH}/src/k8s.io/code-generator
CODE_GENERATOR_TAG=v0.28.8
CONTROLLER_TOOLS_TAG=v0.13.0
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
	${CODE_GENERATOR}/generate-groups.sh "deepcopy,client,informer,lister" \
	github.com/h3poteto/aws-global-accelerator-controller/pkg/client github.com/h3poteto/aws-global-accelerator-controller/pkg/apis \
	endpointgroupbinding:v1alpha1 \
    -h boilerplate.go.txt

code-generator:
ifeq (, $(wildcard ${CODE_GENERATOR}))
	git clone https://github.com/kubernetes/code-generator.git ${CODE_GENERATOR} -b ${CODE_GENERATOR_TAG} --depth 1
endif


manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=global-accelerator-manager-role webhook paths=./... output:crd:artifacts:config=./config/crd/ output:webhook:artifacts:config=./config/webhook/


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

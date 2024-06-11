# How to run E2E tests in local
## Setup kind

Install kind, please refer: https://kind.sigs.k8s.io/

And bootstrap a cluster.

```
$ K8S_VERSION=1.29.4 ./hack/kind-with-registry.sh
```

Install cert-manager

```
$ kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.15.0/cert-manager.crds.yaml
$ kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.15.0/cert-manager.yaml
```

## Build docker and push
```
$ export IMAGE_ID=localhost:5000/aws-global-accelerator-controller
$ docker build . --tag ${IMAGE_ID}:e2e
$ docker push ${IMAGE_ID}:e2e
```

## Execute e2e test
```
$ go install -mod=mod github.com/onsi/ginkgo/v2/ginkgo
$ export WEBHOOK_IMAGE=${IMAGE_ID}:e2e
$ ginkgo -r ./e2e
```

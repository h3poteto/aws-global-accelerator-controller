# How to run E2E tests in local

## Pre-requirements
- [kops](https://github.com/kubernetes/kops)
- [ginkgo v2](https://github.com/onsi/ginkgo)

## Prepare a Kubernetes Cluster
Please rewrite `e2e/cluster.yaml` for your environment, and create a cluster.

```
$ kops create -f e2e/cluster.yaml
$ kops create secret --name $CLUSTER_NAME sshpublickey admin -i ~/.ssh/id_rsa.pub
$ kops update cluster --name $CLUSTER_NAME --yes --admin
$ kops validate cluster --name $CLUSTER_NAME --wait 10m
```

Wait until creating the cluster.

## Run E2E test

```
$ E2E_HOSTNAME=foo.h3poteto-test.dev E2E_MANAGER_IMAGE=ghcr.io/h3poteto/aws-global-accelerator-controller:latest ginkgo -r ./e2e
```

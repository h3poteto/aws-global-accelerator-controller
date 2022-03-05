[![Test](https://github.com/h3poteto/aws-global-accelerator-controller/actions/workflows/test.yml/badge.svg)](https://github.com/h3poteto/aws-global-accelerator-controller/actions/workflows/test.yml)
[![Docker](https://github.com/h3poteto/aws-global-accelerator-controller/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/h3poteto/aws-global-accelerator-controller/actions/workflows/docker-publish.yml)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/h3poteto/aws-global-accelerator-controller)](https://github.com/h3poteto/aws-global-accelerator-controller/releases)
[![Renovate](https://img.shields.io/badge/renovate-enabled-brightgreen.svg)](https://renovatebot.com)
![GitHub](https://img.shields.io/github/license/h3poteto/aws-global-accelerator-controller)

# AWS Global Accelerator Controller
AWS Global Accelerator Controller is a controller to manage Global Accelerator for a Kubenretes cluster. The features are

- Create Global Accelerator for the Load Balancer which is created by Service `type: LoadBalancer`.
- You can control the behavior using Service's annotations
- Create Route53 records associated to Global Accelerator (future work)


## Install
You can install this controller using helm.

```
$ helm repo add h3poteto-stable https://h3poteto.github.io/charts/stable
$ helm install global-accelerator-controller --namespace kube-system h3poteto-stable/aws-global-accelerator-controller
```

### Setup IAM Policy
This controller requires these permissions, so please assign this policy to the controller pod using [IRSA](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html), [kube2iam](https://github.com/jtblin/kube2iam) or [kiam](https://github.com/uswitch/kiam).

```json
{
    "Statement": [
        {
            "Action": [
                "elasticloadbalancing:DescribeLoadBalancers",
                "globalaccelerator:DescribeAccelerator",
                "globalaccelerator:ListAccelerators",
                "globalaccelerator:ListTagsForResource",
                "globalaccelerator:CreateAccelerator",
                "globalaccelerator:DeleteAccelerator",
                "globalaccelerator:ListListeners",
                "globalaccelerator:CreateListener",
                "globalaccelerator:UpdateListener",
                "globalaccelerator:DeleteListener",
                "globalaccelerator:ListEndpointGroups",
                "globalaccelerator:CreateEndpointGroup",
                "globalaccelerator:UpdateEndpointGroup",
                "globalaccelerator:DeleteEndpointGroup"
            ],
            "Effect": "Allow",
            "Resource": "*"
        }
    ],
    "Version": "2012-10-17"
}
```

## Usage

Please specify an annotation `h3poteto.dev/aws-global-accelerator-enabled: "yes"` to your service.

```yaml
apiVersion: v1
kind: Service
metadata:
  annotations:
    h3poteto.dev/aws-global-accelerator-enabled: "yes"
    service.beta.kubernetes.io/aws-load-balancer-backend-protocol: tcp
    service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled: "true"
    service.beta.kubernetes.io/aws-load-balancer-type: nlb
  name: h3poteto-test
  namespace: default
spec:
  externalTrafficPolicy: Local
  ports:
  - name: http
    port: 80
    protocol: TCP
    targetPort: 80
  - name: https
    port: 443
    protocol: TCP
    targetPort: 443
  selector:
    app: h3poteto
  sessionAffinity: None
  type: LoadBalancer

```

Notice: If the service is not `type: LoadBalancer`, this controller does nothing.


## Development
```
$ export KUBECONFIG=$HOME/.kube/config
$ go run ./main.go controller --v=4
```

## License
The software is available as open source under the terms of the [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0).

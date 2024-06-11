package fixtures

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	utilpointer "k8s.io/utils/pointer"
)

func WebhookDeployment(name, ns, image, secretName string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				"app": "webhook",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utilpointer.Int32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "webhook",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "webhook",
					},
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "webhook-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: secretName,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "webhook",
							Image: image,
							Args: []string{
								"/aws-global-accelerator-controller",
								"webhook",
								"--tls-cert-file=/etc/webhook/certs/" + "tls.crt", // CertManager generates a secret with this name
								"--tls-private-key-file=/etc/webhook/certs/" + "tls.key",
							},
							Resources: corev1.ResourceRequirements{
								Limits: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceMemory: {
										Format: resource.Format("500Mi"),
									},
									corev1.ResourceCPU: {
										Format: resource.Format("1000m"),
									},
								},
								Requests: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceMemory: {
										Format: resource.Format("200Mi"),
									},
									corev1.ResourceCPU: {
										Format: resource.Format("100m"),
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "webhook-certs",
									ReadOnly:  true,
									MountPath: "/etc/webhook/certs",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "https",
									ContainerPort: 8443,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(8443),
									},
								},
								InitialDelaySeconds: 30,
								TimeoutSeconds:      60,
								PeriodSeconds:       20,
								SuccessThreshold:    1,
								FailureThreshold:    4,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(8443),
									},
								},
								InitialDelaySeconds: 30,
								TimeoutSeconds:      60,
								PeriodSeconds:       10,
								SuccessThreshold:    2,
								FailureThreshold:    2,
							},
							ImagePullPolicy: corev1.PullAlways,
						},
					},
				},
			},
		},
	}

}

func WebhookService(name, ns string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": "webhook",
			},
			Type: corev1.ServiceTypeClusterIP,
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{},
			},
			Conditions: []metav1.Condition{},
		},
	}
}

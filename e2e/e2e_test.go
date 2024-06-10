package e2e_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/h3poteto/aws-global-accelerator-controller/e2e/pkg/fixtures"
	"github.com/h3poteto/aws-global-accelerator-controller/e2e/pkg/util"
	ownclientset "github.com/h3poteto/aws-global-accelerator-controller/pkg/client/clientset/versioned"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/fixture"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	utilpointer "k8s.io/utils/pointer"
)

var (
	cfg       *rest.Config
	ownClient *ownclientset.Clientset
	client    *kubernetes.Clientset
	ns        = "kube-system"
)

var _ = BeforeSuite(func() {
	configfile := os.Getenv("KUBECONFIG")
	if configfile == "" {
		configfile = "$HOME/.kube/config"
	}
	var err error
	cfg, err = clientcmd.BuildConfigFromFlags("", os.ExpandEnv(configfile))
	Expect(err).ShouldNot(HaveOccurred())

	client, err = kubernetes.NewForConfig(cfg)
	Expect(err).ShouldNot(HaveOccurred())

	ownClient, err = ownclientset.NewForConfig(cfg)
	Expect(err).ShouldNot(HaveOccurred())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	err = waitUntilReady(ctx, client)
	Expect(err).ShouldNot(HaveOccurred())
})

var _ = Describe("E2E", func() {
	BeforeEach(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		_, err := client.CoreV1().Namespaces().Get(ctx, "system", metav1.GetOptions{})
		if err != nil {
			_, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system",
				},
			}, metav1.CreateOptions{})
			if err != nil {
				panic(err)
			}
		}

		// Apply CRDs
		if err := util.ApplyCRD(ctx, cfg); err != nil {
			panic(err)
		}
		// Apply webhook manifests
		if err := util.ApplyWebhook(ctx, cfg); err != nil {
			panic(err)
		}
		// Deployment, service, Certificate, Issuer
		if err := applyWebhook(ctx, cfg, client); err != nil {
			panic(err)
		}
	})

	AfterEach(func() {
		// Delete all endpointgroupbinding
		ownClient.OperatorV1alpha1().EndpointGroupBindings(ns).DeleteCollection(context.Background(), metav1.DeleteOptions{}, metav1.ListOptions{})
	})
	It("Changing ARN", func() {
		resource := fixture.EndpointGroupBinding(false, "example", utilpointer.Int32(100), "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcd1234")
		_, err := ownClient.OperatorV1alpha1().EndpointGroupBindings(ns).Create(context.Background(), &resource, metav1.CreateOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		current, err := ownClient.OperatorV1alpha1().EndpointGroupBindings(ns).Get(context.Background(), resource.Name, metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		current.Spec.EndpointGroupArn = "arn:aws:globalaccelerator::123456789012:accelerator/5678efgh-efgh-5678-efgh-5678efgh5678"
		_, err = ownClient.OperatorV1alpha1().EndpointGroupBindings(current.Namespace).Update(context.Background(), current, metav1.UpdateOptions{})
		Expect(err).Should(HaveOccurred())
		Expect(strings.Contains(err.Error(), "Spec.EndpointGroupArn is immutable")).Should(BeTrue())
	})
	It("Changing weight", func() {
		resource := fixture.EndpointGroupBinding(false, "example", utilpointer.Int32(100), "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcd1234")
		_, err := ownClient.OperatorV1alpha1().EndpointGroupBindings(ns).Create(context.Background(), &resource, metav1.CreateOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		current, err := ownClient.OperatorV1alpha1().EndpointGroupBindings(ns).Get(context.Background(), resource.Name, metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		current.Spec.Weight = utilpointer.Int32(200)
		_, err = ownClient.OperatorV1alpha1().EndpointGroupBindings(current.Namespace).Update(context.Background(), current, metav1.UpdateOptions{})
		Expect(err).ShouldNot(HaveOccurred())
	})

})

func waitUntilReady(ctx context.Context, client *kubernetes.Clientset) error {
	klog.Info("Waiting until kubernetes cluster is ready")
	err := wait.Poll(10*time.Second, 10*time.Minute, func() (bool, error) {
		nodeList, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to list nodes: %v", err)
		}
		if len(nodeList.Items) == 0 {
			klog.Warningf("node does not exist yet")
			return false, nil
		}
		for i := range nodeList.Items {
			n := &nodeList.Items[i]
			if !nodeIsReady(n) {
				klog.Warningf("node %s is not ready yet", n.Name)
				return false, nil
			}
		}
		klog.Info("all nodes are ready")
		return true, nil
	})
	return err
}

func nodeIsReady(node *corev1.Node) bool {
	for i := range node.Status.Conditions {
		con := &node.Status.Conditions[i]
		if con.Type == corev1.NodeReady && con.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func applyWebhook(ctx context.Context, cfg *rest.Config, client *kubernetes.Clientset) error {
	secretName := "webhook-certs"
	serviceName := "webhook-service" // defined in webhook manifests.yaml
	namespace := "system"            // defined in webhook manifests.yaml

	// Apply Issuer
	err := util.ApplyIssuer(ctx, cfg, namespace)
	if err != nil {
		return err
	}
	// Apply Certificate
	err = util.ApplyCertificate(ctx, cfg, namespace, serviceName, secretName)
	if err != nil {
		return err
	}
	// Apply Deployment
	image := os.Getenv("WEBHOOK_IMAGE")
	deploy := fixtures.WebhookDeployment("webhook", namespace, image, secretName)
	if _, err := client.AppsV1().Deployments("system").Get(ctx, deploy.Name, metav1.GetOptions{}); err != nil {
		_, err = client.AppsV1().Deployments("system").Create(ctx, deploy, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}
	// Apply Service
	service := fixtures.WebhookService(serviceName, namespace)
	if _, err := client.CoreV1().Services("system").Get(ctx, service.Name, metav1.GetOptions{}); err != nil {
		_, err = client.CoreV1().Services("system").Create(ctx, service, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}
	wait.PollUntilContextTimeout(ctx, 10*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		deployment, err := client.AppsV1().Deployments("system").Get(ctx, deploy.Name, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("Failed to get deployment: %v", err)
			return false, nil
		}
		if deployment.Status.ReadyReplicas > 0 {
			return true, nil
		}
		klog.Info("Waiting for deployment ready")
		return false, nil
	})
	return nil
}

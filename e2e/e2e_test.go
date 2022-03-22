package e2e_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/h3poteto/aws-global-accelerator-controller/e2e/pkg/fixtures"
	cloudaws "github.com/h3poteto/aws-global-accelerator-controller/pkg/cloudprovider/aws"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

var (
	cfg        *rest.Config
	kubeClient *kubernetes.Clientset
	namespace  string
	hostname   string
)

var _ = BeforeSuite(func() {
	configfile := os.Getenv("KUBECONFIG")
	if configfile == "" {
		configfile = "$HOME/.kube/config"
	}
	var err error
	cfg, err = clientcmd.BuildConfigFromFlags("", os.ExpandEnv(configfile))
	Expect(err).ShouldNot(HaveOccurred())
	kubeClient, err = kubernetes.NewForConfig(cfg)
	Expect(err).ShouldNot(HaveOccurred())
	err = waitUntilReady(context.Background(), kubeClient)
	Expect(err).ShouldNot(HaveOccurred())
	hostname = os.Getenv("E2E_HOSTNAME")
	Expect(hostname).ShouldNot(BeEmpty(), "Env var E2E_HOSTNAME is required")
	namespace = os.Getenv("E2E_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}
})

var _ = Describe("E2E", func() {
	BeforeEach(func() {
		ctx := context.Background()
		image := os.Getenv("E2E_MANAGER_IMAGE")
		Expect(image).ShouldNot(BeEmpty(), "Env var E2E_MANAGER_IMAGE is required")
		err := fixtures.ApplyClusterRole(ctx, cfg)
		Expect(err).ShouldNot(HaveOccurred())
		sa, crb, dep := fixtures.NewManagerManifests(namespace, "aws-global-accelerator-controller", image)
		_, err = kubeClient.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		_, err = kubeClient.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		_, err = kubeClient.AppsV1().Deployments(namespace).Create(ctx, dep, metav1.CreateOptions{})
		Expect(err).ShouldNot(HaveOccurred())

		DeferCleanup(func() {
			_ = kubeClient.AppsV1().Deployments(namespace).Delete(ctx, dep.Name, metav1.DeleteOptions{})
			_ = kubeClient.RbacV1().ClusterRoleBindings().Delete(ctx, crb.Name, metav1.DeleteOptions{})
			_ = kubeClient.CoreV1().ServiceAccounts(namespace).Delete(ctx, sa.Name, metav1.DeleteOptions{})
			_ = fixtures.DeleteClusterRole(ctx, cfg)
		})

		err = wait.Poll(5*time.Second, 2*time.Minute, func() (bool, error) {
			deploy, err := kubeClient.AppsV1().Deployments(namespace).Get(ctx, dep.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			klog.Infof("Deployment %s/%s is available %d/%d", deploy.Namespace, deploy.Name, deploy.Status.AvailableReplicas, *deploy.Spec.Replicas)
			if deploy.Status.AvailableReplicas == *deploy.Spec.Replicas && deploy.Status.ReadyReplicas == *deploy.Spec.Replicas {
				return true, nil
			}
			return false, nil
		})
		Expect(err).ShouldNot(HaveOccurred(), "Manager pods are not running")

	})
	Describe("Service type Load Balancer", func() {
		It("Global Accelerator and Route53 should be created", func() {
			ctx := context.Background()
			svc := fixtures.NewNLBService(namespace, "e2e-test", hostname)
			_, err := kubeClient.CoreV1().Services(namespace).Create(ctx, svc, metav1.CreateOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			DeferCleanup(func() {
				kubeClient.CoreV1().Services(svc.Namespace).Delete(ctx, svc.Name, metav1.DeleteOptions{})
				// TODO: wait until cleanup
			})

			By("Wait until LoadBalancer is created", func() {
				err = wait.PollImmediate(10*time.Second, 5*time.Minute, func() (bool, error) {
					currentService, err := kubeClient.CoreV1().Services(namespace).Get(ctx, svc.Name, metav1.GetOptions{})
					if err != nil {
						return false, err
					}
					if len(currentService.Status.LoadBalancer.Ingress) > 0 {
						return true, nil
					}
					klog.Infof("%s/%s does not have loadBalancer yet", currentService.Namespace, currentService.Name)
					return false, nil
				})
				Expect(err).ShouldNot(HaveOccurred())
			})

			svc, err = kubeClient.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			lbName, region, err := cloudaws.GetLBNameFromHostname(svc.Status.LoadBalancer.Ingress[0].Hostname)
			Expect(err).ShouldNot(HaveOccurred())
			cloud := cloudaws.NewAWS(region)

			By("Wait until Global Accelerator is created", func() {
				lb, err := cloud.GetLoadBalancer(ctx, lbName)
				Expect(err).ShouldNot(HaveOccurred())

				err = wait.Poll(10*time.Second, 5*time.Minute, func() (bool, error) {
					accelerators, err := cloud.ListGlobalAcceletaroByResource(ctx, "service", svc.Namespace, svc.Name)
					if err != nil {
						return false, err
					}
					if len(accelerators) == 0 {
						klog.Infof("There is no accelerator related Service %s/%s", svc.Namespace, svc.Name)
						return false, nil
					}
					for _, accelerator := range accelerators {
						listener, err := cloud.GetListener(ctx, *accelerator.AcceleratorArn)
						if err != nil {
							return false, err
						}
						endpoint, err := cloud.GetEndpointGroup(ctx, *listener.ListenerArn)
						if err != nil {
							return false, err
						}
						for _, d := range endpoint.EndpointDescriptions {
							if *d.EndpointId == *lb.LoadBalancerArn {
								return true, nil
							}
						}
					}
					klog.Infof("There is no endpoint group related Service %s/%s", svc.Namespace, svc.Name)
					return false, nil
				})
				Expect(err).ShouldNot(HaveOccurred())
			})

			hostnames := strings.Split(hostname, ",")

			By("Wait until Route53 record is created", func() {
				accelerators, err := cloud.ListGlobalAcceleratorByHostname(ctx, svc.Status.LoadBalancer.Ingress[0].Hostname, "service", svc.Namespace, svc.Name)
				Expect(err).ShouldNot(HaveOccurred())
				acceleratorHostname := *accelerators[0].DnsName

				for _, h := range hostnames {
					hostedZone, err := cloud.GetHostedZone(ctx, h)
					Expect(err).ShouldNot(HaveOccurred())

					err = wait.PollImmediate(10*time.Second, 5*time.Minute, func() (bool, error) {
						records, err := cloud.FindOwneredARecordSets(ctx, hostedZone, cloudaws.Route53OwnerValue("service", svc.Namespace, svc.Name))
						if err != nil {
							return false, err
						}
						if len(records) == 0 {
							klog.Infof("There is no route53 record related Service %s/%s", svc.Namespace, svc.Name)
							return false, nil
						}
						for _, record := range records {
							if record.AliasTarget != nil && *record.AliasTarget.DNSName == acceleratorHostname+"." {
								return true, nil
							}
						}
						klog.Infof("There is no route53 record related Global Accelerator: %s", acceleratorHostname)
						return false, nil
					})
					Expect(err).ShouldNot(HaveOccurred())
				}
			})

			By("Remove resources", func() {
				err := kubeClient.CoreV1().Services(svc.Namespace).Delete(ctx, svc.Name, metav1.DeleteOptions{})
				Expect(err).ShouldNot(HaveOccurred())

				for _, h := range hostnames {
					hostedZone, err := cloud.GetHostedZone(ctx, h)
					Expect(err).ShouldNot(HaveOccurred())
					err = wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) {
						records, err := cloud.FindOwneredARecordSets(ctx, hostedZone, cloudaws.Route53OwnerValue("service", svc.Namespace, svc.Name))
						if err != nil {
							return false, err
						}
						if len(records) == 0 {
							return true, nil
						}
						klog.Info("There are some Route53 records")
						return false, nil
					})
					Expect(err).ShouldNot(HaveOccurred())
				}

				err = wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) {
					accelerators, err := cloud.ListGlobalAcceletaroByResource(ctx, "service", svc.Namespace, svc.Name)
					if err != nil {
						return false, err
					}
					if len(accelerators) == 0 {
						return true, nil
					}
					klog.Info("There are some Global Accelerators")
					return false, nil
				})
				Expect(err).ShouldNot(HaveOccurred())

			})
		})
	})
})

func waitUntilReady(ctx context.Context, client *kubernetes.Clientset) error {
	klog.Info("Waiting until kubernetes cluster is ready")
	err := wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) {
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

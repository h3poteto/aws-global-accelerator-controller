package globalaccelerator

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *GlobalAcceleratorController) prepareCorrespondence(ctx context.Context) (map[string]string, error) {
	cm, err := c.kubeclient.CoreV1().ConfigMaps(c.namespace).Get(ctx, dataConfigMap, metav1.GetOptions{})
	if kerrors.IsNotFound(err) {
		return make(map[string]string), nil
	} else if err != nil {
		return nil, err
	}
	return parseCorrespondence(cm)
}

func parseCorrespondence(cm *corev1.ConfigMap) (map[string]string, error) {
	if cm.Data == nil {
		return make(map[string]string), nil
	}
	return cm.Data, nil
}

func (c *GlobalAcceleratorController) updateCorrespondence(ctx context.Context, correspondence map[string]string) error {
	cm, err := c.kubeclient.CoreV1().ConfigMaps(c.namespace).Get(ctx, dataConfigMap, metav1.GetOptions{})
	if kerrors.IsNotFound(err) {
		if _, err := c.createCorrespondence(ctx, correspondence); err != nil {
			return err
		}
		return nil
	} else if err != nil {
		return err
	}
	cm.Data = correspondence
	_, err = c.kubeclient.CoreV1().ConfigMaps(c.namespace).Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

func (c *GlobalAcceleratorController) createCorrespondence(ctx context.Context, correspondence map[string]string) (*corev1.ConfigMap, error) {
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dataConfigMap,
			Namespace: c.namespace,
		},
		Data: correspondence,
	}
	return c.kubeclient.CoreV1().ConfigMaps(c.namespace).Create(ctx, &cm, metav1.CreateOptions{})
}

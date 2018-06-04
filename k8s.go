package kubelogtail

import (
	"github.com/pkg/errors"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func kubeClient(kubeconfig string) (*kubernetes.Clientset, error) {
	// TODO: allow overriding context
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create kubernetes config")
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create kubernetws client")
	}
	return clientset, nil
}

//we should really do a watch, but this is fine for now
func getPods(clientset *kubernetes.Clientset, namespace string, selector string) (*v1.PodList, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to list pods")
	}
	return pods, nil
}

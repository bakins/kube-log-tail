package kubelogtail

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"

	// import for auth providers
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

// OptionsFunc is a function passed to new for setting options on a new KubeLogTail.
type OptionsFunc func(*KubeLogTail) error

// KubeLogTail is a wrapper for discovering and printing logs from Kubernetes containers.
type KubeLogTail struct {
	kubeconfig  string
	clientset   *kubernetes.Clientset
	ctx         context.Context
	cancel      context.CancelFunc
	refreshTime time.Duration
	namespace   string
	selector    string
	pods        map[string]*podTail
	colorMode   string
	color       *logColorPrint
}

// New creates a new KubeLogTail
func New(options ...OptionsFunc) (*KubeLogTail, error) {
	k := &KubeLogTail{
		refreshTime: time.Second * 10,
		namespace:   "default",
		pods:        make(map[string]*podTail),
		colorMode:   "pod",
	}

	k.ctx, k.cancel = context.WithCancel(context.Background())
	for _, f := range options {
		if err := f(k); err != nil {
			return nil, errors.Wrapf(err, "failed to process options")
		}
	}

	client, err := kubeClient(k.kubeconfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create kubernetes client from %s", k.kubeconfig)
	}
	k.clientset = client

	c, err := newLogColorPrint(k.colorMode)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create color functions")
	}
	k.color = c

	return k, nil
}

// SetKubeConfig creates a function that will set the kubeconfig.
// Generally, only used when create a new KubeLogTail.
func SetKubeConfig(kubeconfig string) OptionsFunc {
	return func(k *KubeLogTail) error {
		k.kubeconfig = kubeconfig
		return nil
	}
}

// SetRefreshTime creates a function that will set the pod refresh time.
// Generally, only used when create a new KubeLogTail.
func SetRefreshTime(duration time.Duration) OptionsFunc {
	return func(k *KubeLogTail) error {
		k.refreshTime = duration
		return nil
	}
}

// SetNamespace creates a function that will set the namespace for pods.
// a blank string indicates all namespaces
// Generally, only used when create a new KubeLogTail.
func SetNamespace(namespace string) OptionsFunc {
	return func(k *KubeLogTail) error {
		k.namespace = namespace
		return nil
	}
}

// SetLabelSelector creates a function that will set the label selecotr for
// listing pods.
// Generally, only used when create a new KubeLogTail.
func SetLabelSelector(query string) OptionsFunc {
	return func(k *KubeLogTail) error {
		l, err := labels.Parse(query)
		if err != nil {
			return errors.Wrapf(err, "failed to parse label selector: \"%s\"", query)
		}
		k.selector = l.String()
		return nil
	}
}

// SetColorMode creates a function that will set the color print mode.
// Generally, only used when create a new KubeLogTail.
func SetColorMode(mode string) OptionsFunc {
	return func(k *KubeLogTail) error {
		k.colorMode = mode
		return nil
	}
}

// Stop triggers the kubelog tail to stop processing.
func (k *KubeLogTail) Stop() {
	k.cancel()
}

// Run discovers the pods and tails the container logs.
// This generally does not return.
func (k *KubeLogTail) Run() error {
	//run first time
	if err := k.processPods(); err != nil {
		return errors.Wrapf(err, "failed to process pods for logs")
	}

	t := time.NewTicker(k.refreshTime)
LOOP:
	for {
		select {
		case <-t.C:
			if err := k.processPods(); err != nil {
				fmt.Println(errors.Wrapf(err, "failed to process pods for logs"))
			}
		case <-k.ctx.Done():
			// this probably isn't actually needed
			t.Stop()
			break LOOP
		}
	}
	// cancel should propogate to all the children
	return nil
}

func (k *KubeLogTail) processPods() error {
	pods, err := getPods(k.clientset, k.namespace, k.selector)
	if err != nil {
		return errors.Wrapf(err, "failed to list pods")
	}

	oldPods := make(map[string]bool)
	for key := range k.pods {
		oldPods[key] = true
	}

	foundPods := make(map[string]*v1.Pod)
	for i := range pods.Items {
		p := &pods.Items[i]
		key := fmt.Sprintf("%s/%s", p.GetNamespace(), p.GetName())
		foundPods[key] = p
	}

	// start new pods
	for key, pod := range foundPods {
		_, ok := k.pods[key]
		if ok {
			continue
		}
		t := k.newPodTail(pod)
		k.pods[key] = t

		// TODO: channel for errors?
		go func() {
			err := t.tail()
			if err != nil {
				fmt.Println(err)
			}
		}()
	}

	// stop old ones
	for key := range oldPods {
		_, ok := foundPods[key]
		if ok {
			continue
		}
		t, ok := k.pods[key]
		if !ok {
			// should never happen
			continue
		}
		t.stop()
	}
	return nil
}

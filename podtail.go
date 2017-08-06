package kubelogtail

import (
	"bufio"
	"context"
	"fmt"
	"sync"

	"github.com/pkg/errors"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
)

type colorFuncGet func() colorFunc

type podTail struct {
	ctx          context.Context
	cancel       context.CancelFunc
	namespace    string
	name         string
	containers   []string
	client       rest.Interface
	colorFuncGet colorFuncGet
}

func (k *KubeLogTail) newPodTail(pod *v1.Pod) *podTail {
	p := podTail{
		namespace:    pod.GetNamespace(),
		name:         pod.GetName(),
		client:       k.clientset.CoreV1().RESTClient(),
		colorFuncGet: k.color.getColor,
	}

	p.ctx, p.cancel = context.WithCancel(k.ctx)

	for _, c := range pod.Spec.Containers {
		p.containers = append(p.containers, c.Name)
	}
	return &p
}

func (p *podTail) stop() {
	p.cancel()
}

// get all containers and tail. generally does not return.
func (p *podTail) tail() error {
	var wg sync.WaitGroup
	for _, c := range p.containers {
		var err error
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			err = p.tailContainer(name)
		}(c)
		if err != nil {
			return errors.Wrapf(err, "failed tailing logs for pod %s/%s", p.namespace, p.name)
		}
	}
	wg.Wait()
	return nil
}

func (p *podTail) tailContainer(name string) error {
	// see https://stackoverflow.com/a/32984298
	// and https://kubernetes.io/docs/api-reference/v1.6/#-strong-misc-operations-strong--71

	// TODO: add retry logic on timeout on long poll.
	label := fmt.Sprintf("%s/%s/%s", p.namespace, p.name, name)
	printer := p.colorFuncGet()

	// TODO: allow time since and number of lines options?
	req := p.client.Get().
		Namespace(p.namespace).
		Name(p.name).
		Resource("pods").
		SubResource("log").
		Param("follow", "true").
		Param("container", name).
		Context(p.ctx)

	readCloser, err := req.Stream()
	if err != nil {
		return errors.Wrapf(err, "unable to stream logs for %s", label)
	}

	defer func() { _ = readCloser.Close() }()

	scanner := bufio.NewScanner(readCloser)
	for scanner.Scan() {
		printer(label, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return errors.Wrapf(err, "error scanning for lines for %s", label)
	}
	return nil
}

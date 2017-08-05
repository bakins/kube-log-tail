package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fatih/color"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const cmdVersion = "0.1.0"

var (
	version       bool
	kubeconfig    string
	namespace     string
	labelSelector string
	refreshTime   time.Duration
)

func main() {
	log.SetFlags(-1)

	rootCmd := &cobra.Command{
		Use:   "kube-log-tail",
		Short: "tail kubernetes pod logs",
		Run:   runTail,
	}

	f := rootCmd.PersistentFlags()

	f.BoolVarP(&version, "version", "v", false, "display the current version")
	f.StringVar(&kubeconfig, "kubeconfig", defaultKubeconfig(), "path to kubeconfig")
	f.StringVarP(&namespace, "namespace", "n", "default", "namespace for pods. use \"\" for all namespaces")
	f.StringVarP(&labelSelector, "selector", "l", "", "label selector for pods")
	f.DurationVarP(&refreshTime, "refresh", "r", time.Second*10, "how often to refresh the list of pods")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func defaultKubeconfig() string {
	dir, err := homedir.Dir()
	if err != nil {
		log.Fatalf("unable to determine home directory: %s", err)
	}
	return filepath.Join(dir, ".kube", "config")
}

type colorFunc func(string, string)

type podWatcher struct {
	clientset    *kubernetes.Clientset
	ctx          context.Context
	cancel       context.CancelFunc
	refreshTime  time.Duration
	namespace    string
	selector     string
	pods         map[string]*podTail
	colorFuncs   []colorFunc
	currentColor int //off set into colors
	sync.Mutex       // for colors
}

type fmtPrinter func(format string, a ...interface{}) string

func (w *podWatcher) generateColors() {
	w.Lock()
	defer w.Unlock()

	colors := []fmtPrinter{color.BlueString, color.CyanString, color.YellowString, color.RedString, color.MagentaString}
	for i := range colors {
		f := colors[i]
		cf := func(label, line string) {
			fmt.Println(f(label), line)
		}
		w.colorFuncs = append(w.colorFuncs, cf)
	}
}

func (w *podWatcher) getColor() colorFunc {
	w.Lock()
	defer w.Unlock()
	w.currentColor++
	if w.currentColor >= len(w.colorFuncs) {
		w.currentColor = 0
	}
	return w.colorFuncs[w.currentColor]
}

func (w *podWatcher) processPods() error {
	pods, err := getPods(w.clientset, w.namespace, w.selector)
	if err != nil {
		return errors.Wrapf(err, "failed to list pods")
	}

	oldPods := make(map[string]bool)
	for k, _ := range w.pods {
		oldPods[k] = true
	}

	foundPods := make(map[string]*v1.Pod)
	for _, p := range pods.Items {
		key := fmt.Sprintf("%s/%s", p.GetNamespace(), p.GetName())
		foundPods[key] = &p
	}

	// start new pods
	for k, v := range foundPods {
		_, ok := w.pods[k]
		if ok {
			continue
		}
		fmt.Println("adding", k)
		ctx, cancel := context.WithCancel(w.ctx)
		t := &podTail{
			pod:     v,
			ctx:     ctx,
			cancel:  cancel,
			watcher: w,
		}

		w.pods[k] = t
		// TODO: channel for errors?
		go func() {
			err := t.tailLogs(w.clientset.CoreV1Client.RESTClient())
			if err != nil {
				fmt.Println(err)
			}
		}()
	}

	// stop old ones
	for k, _ := range oldPods {
		_, ok := foundPods[k]
		if ok {
			continue
		}
		t, ok := w.pods[k]
		if !ok {
			// should never happen
			continue
		}
		t.stop()
	}
	return nil
}

func (w *podWatcher) stop() {
	w.cancel()
}

func (w *podWatcher) run() error {
	//run first time
	if err := w.processPods(); err != nil {
		return errors.Wrapf(err, "failed to process pods for logs")
	}

	t := time.NewTicker(w.refreshTime)
LOOP:
	for {
		select {
		case <-t.C:
			if err := w.processPods(); err != nil {
				fmt.Println(errors.Wrapf(err, "failed to process pods for logs"))
			}
		case <-w.ctx.Done():
			// this probably isn't actually needed
			t.Stop()
			break LOOP
		}
	}
	// cancel should propogate to all the children
	return nil
}

type podTail struct {
	ctx     context.Context
	cancel  context.CancelFunc
	pod     *v1.Pod
	watcher *podWatcher
	// TODO: add colors
}

func (p *podTail) stop() {
	p.cancel()
}

func (p *podTail) tailContainer(client restclient.Interface, name string) error {
	label := fmt.Sprintf("%s/%s/%s", p.pod.GetNamespace(), p.pod.GetName(), name)

	printer := p.watcher.getColor()
	req := client.Get().
		Namespace(p.pod.GetNamespace()).
		Name(p.pod.GetName()).
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
		//		fmt.Println("printing", label)
		printer(label, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return errors.Wrapf(err, "error scanning for lines for %s", label)
	}
	return nil

}

// TODO: option to show previous logs as well?
// time based? sinceTime or tailLines
func (p *podTail) tailLogs(client restclient.Interface) error {
	// from https://stackoverflow.com/a/32984298
	// https://kubernetes.io/docs/api-reference/v1.6/#-strong-misc-operations-strong--71
	var wg sync.WaitGroup
	for _, c := range p.pod.Spec.Containers {
		var err error
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			err = p.tailContainer(client, name)
		}(c.Name)
		if err != nil {
			return errors.Wrapf(err, "failed tailing logs for pod %s/%s", p.pod.GetNamespace(), p.pod.GetName())
		}
	}
	wg.Wait()
	return nil
}

//type
func runTail(cmd *cobra.Command, args []string) {
	if version {
		fmt.Printf("kube-log-tail version: %s\n", cmdVersion)
		os.Exit(0)
	}

	selector, err := labels.Parse(labelSelector)
	if err != nil {
		log.Fatalf("failed to parse label selector: %s", err)
	}
	labelSelector = selector.String()

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("failed to create kubernetes config: %s", err)
	}

	clientset, err := kubeClient(config)
	if err != nil {
		log.Fatalf("failed to create kubernetes client: %s", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	w := &podWatcher{
		clientset:   clientset,
		ctx:         ctx,
		cancel:      cancel,
		refreshTime: refreshTime,
		namespace:   namespace,
		selector:    labelSelector,
		pods:        make(map[string]*podTail),
	}

	w.generateColors()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		w.stop()
	}()

	if err := w.run(); err != nil {
		log.Fatal(err)
	}
}

func kubeClient(config *restclient.Config) (*kubernetes.Clientset, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create kubernets client")
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

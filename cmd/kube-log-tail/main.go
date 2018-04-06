// Package main runs the command and handles command line options
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	kubelogtail "github.com/bakins/kube-log-tail"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
)

const cmdVersion = "0.2.0"

var (
	version       bool
	kubeconfig    string
	namespace     string
	labelSelector string
	colorMode     string
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
	f.StringVarP(&colorMode, "colored-output", "k", "pod", "use colored output (pod|line|off)")
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func runTail(cmd *cobra.Command, args []string) {
	if version {
		fmt.Printf("kube-log-tail version: %s\n", cmdVersion)
		os.Exit(0)
	}

	k, err := kubelogtail.New(
		kubelogtail.SetKubeConfig(kubeconfig),
		kubelogtail.SetRefreshTime(refreshTime),
		kubelogtail.SetNamespace(namespace),
		kubelogtail.SetLabelSelector(labelSelector),
		kubelogtail.SetColorMode(colorMode),
	)

	if err != nil {
		log.Fatal(err)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		k.Stop()
	}()

	if err := k.Run(); err != nil {
		log.Fatal(err)
	}
}

func defaultKubeconfig() string {
	dir, err := homedir.Dir()
	if err != nil {
		log.Fatalf("unable to determine home directory: %s", err)
	}
	if os.Getenv("KUBECONFIG") != "" {
		return os.Getenv("KUBECONFIG")
	} else {
		return filepath.Join(dir, ".kube", "config")
	}
}

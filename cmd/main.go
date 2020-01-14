/*
Copyright © 2018 inwinSTACK Inc

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	blended "github.com/inwinstack/blended/generated/clientset/versioned"
	"github.com/inwinstack/ipam/pkg/config"
	"github.com/inwinstack/ipam/pkg/operator"
	"github.com/inwinstack/ipam/pkg/version"
	"github.com/spf13/pflag"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

var (
	cfg        = &config.Config{}
	kubeconfig string
	ver        bool
)

func restConfig(kubeconfig string) (cfg *rest.Config, err error) {
	if kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		cfg, err = rest.InClusterConfig()
	}
	return cfg, nil
}

func versionInfo() []string {
	rv := []string{
		fmt.Sprintf("Inwinstack-IPAM Version: %s", version.GetVersion()),
		fmt.Sprintf("Go Version: %s", runtime.Version()),
		fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH),
	}
	return rv
}

func main() {
	defer klog.Flush()
	klog.InitFlags(flag.CommandLine)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.StringVarP(&kubeconfig, "kubeconfig", "", "", "Absolute path to the kubeconfig file.")
	pflag.IntVarP(&cfg.Threads, "threads", "", 2, "Number of worker threads used by the controller.")
	pflag.IntVarP(&cfg.SyncSec, "sync-seconds", "", 30, "Seconds for syncing and retrying objects.")
	pflag.BoolVarP(&ver, "version", "", false, "Display the version")
	pflag.Parse()

	if ver {
		for _, s := range versionInfo() {
			fmt.Fprintln(os.Stdout, s)
		}
		os.Exit(0)
	}

	for _, s := range versionInfo() {
		klog.Infoln(s)
	}

	k8scfg, err := restConfig(kubeconfig)
	if err != nil {
		klog.Fatalf("Error to build kubeconfig: %s", err.Error())
	}

	blendedclient, err := blended.NewForConfig(k8scfg)
	if err != nil {
		klog.Fatalf("Error to build Blended client: %s", err.Error())
	}

	ctx, cancel := context.WithCancel(context.Background())
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	op := operator.New(cfg, blendedclient)
	if err := op.Run(ctx); err != nil {
		klog.Fatalf("Error to serve the operator instance: %s.", err)
	}

	<-signalChan
	cancel()
	op.Stop()
	klog.Infof("Shutdown signal received, exiting...")
}

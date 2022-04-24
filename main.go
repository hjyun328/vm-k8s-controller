/*
Copyright 2017 The Kubernetes Authors.

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
	"net/http"
	"os"
	"time"

	"k8s.io/sample-controller/internal/webhook"

	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"k8s.io/sample-controller/internal/utils"
	"k8s.io/sample-controller/pkg/privatecloud"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	// Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	clientset "k8s.io/sample-controller/pkg/generated/clientset/versioned"
	informers "k8s.io/sample-controller/pkg/generated/informers/externalversions"
	"k8s.io/sample-controller/pkg/signals"
)

const (
	envPodName                   = "POD_NAME"
	envPodNamespace              = "POD_NAMESPACE"
	envWatchNamespace            = "WATCH_NAMESPACE"
	envValidatingWebhookPort     = "VALIDATING_WEBHOOK_PORT"
	envValidatingWebhookCertPath = "VALIDATING_WEBHOOK_CERT_PATH"
	envValidatingWebhookKeyPath  = "VALIDATING_WEBHOOK_KEY_PATH"
	envExporterPort              = "EXPORTER_PORT"
	envLeaderElectionLockName    = "LEADER_ELECTION_LOCK_NAME"
)

const (
	defaultWatchNamespace                  = metav1.NamespaceAll
	defaultVmValidatingWebhookPort         = 8443
	defaultExporterPort                    = 8500
	defaultLeaderElectionLockName          = "sample-controller-lock"
	defaultLeaderElectionRetryAfter        = 10 * time.Second
	defaultLeaderElectionRetryJitterFactor = .2
	//defaultPrivateCloudApiHostPort    = "localhost:8080"
	//defaultPrivateCloudApiHttpTimeout = 5 * time.Second
)

var (
	masterURL  string
	kubeconfig string
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	podName := os.Getenv(envPodName)
	podNamespace := os.Getenv(envPodNamespace)
	watchNamespace := utils.DefaultString(os.Getenv(envWatchNamespace), defaultWatchNamespace)

	validatingWebhookPort := utils.DefaultStringToInt(os.Getenv(envValidatingWebhookPort), defaultVmValidatingWebhookPort)
	validatingWebhookCert := os.Getenv(envValidatingWebhookCertPath)
	validatingWebhookKey := os.Getenv(envValidatingWebhookKeyPath)

	exporterPort := utils.DefaultStringToInt(os.Getenv(envExporterPort), defaultExporterPort)

	leaderElectionLockName := utils.DefaultString(os.Getenv(envLeaderElectionLockName), defaultLeaderElectionLockName)

	//vmApi := privatecloud.DefaultVmApi(defaultPrivateCloudApiHostPort, defaultPrivateCloudApiHttpTimeout)
	vmApi := &privatecloud.FakeVmApi{}

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	// start vm validating webhook in k8s cluster
	if validatingWebhookCert != "" && validatingWebhookKey != "" {
		go startVmValidatingWebhook(vmApi,
			validatingWebhookPort, validatingWebhookCert, validatingWebhookKey, stopCh)
	}

	// start exporter
	go startExporter(exporterPort, stopCh)

	if podName != "" && podNamespace != "" {
		masterURL = ""
		kubeconfig = ""
	}
	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	// start leader election in k8s cluster
	if podName != "" && podNamespace != "" {
		startLeaderElection(podName, podNamespace, leaderElectionLockName, kubeClient, stopCh)
	}

	sampleClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building example clientset: %s", err.Error())
	}

	sampleInformerFactory := informers.NewSharedInformerFactoryWithOptions(sampleClient,
		time.Second*30, informers.WithNamespace(watchNamespace))

	controller := NewController(kubeClient, sampleClient,
		sampleInformerFactory.Samplecontroller().V1alpha1().VMs(),
		vmApi)

	sampleInformerFactory.Start(stopCh)

	if err = controller.Run(1, stopCh); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}
}

func startVmValidatingWebhook(vmApi privatecloud.VmApi, port int, cert string, key string, stopCh <-chan struct{}) {
	klog.Info("start validating webhook server.")
	mux := http.NewServeMux()
	mux.Handle("/", webhook.NewVMValidatingWebHook(vmApi))
	server := http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}
	go func() {
		if err := server.ListenAndServeTLS(cert, key); err != nil {
			klog.Fatal(err)
		}
	}()
	<-stopCh
	if err := server.Shutdown(context.TODO()); err != nil {
		klog.Fatal(err)
	}
}

func startExporter(port int, stopCh <-chan struct{}) {
	klog.Info("start exporter.")
	// TODO: cannot expose reflector metrics
	// https://github.com/kubernetes-sigs/controller-runtime/issues/817
	// https://github.com/kubernetes/kubernetes/pull/74636

	// expose go-client like workqueue, client stats.
	metrics.Registry.MustRegister(
		// expose Go runtime metrics like GC stats, memory stats etc.
		collectors.NewGoCollector(),
		// expose process metrics like CPU, Memory, file descriptor usage etc.
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	handler := promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError,
	})
	mux := http.NewServeMux()
	mux.Handle("/metrics", handler)
	server := http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != nil {
			klog.Fatal(err)
		}
	}()
	<-stopCh
	if err := server.Shutdown(context.TODO()); err != nil {
		klog.Fatal(err)
	}
}

func startLeaderElection(podName string, podNamespace string, lockName string, client kubernetes.Interface, stopCh <-chan struct{}) {
	klog.Info("start leader election.")
	klog.Info("wait for becoming leader.")
	for {
		configMap, err := client.CoreV1().ConfigMaps(podNamespace).Get(
			context.TODO(), lockName, metav1.GetOptions{})
		if err == nil {
			for _, ownerReference := range configMap.OwnerReferences {
				if ownerReference.Name == podName {
					klog.Info("this controller has been restarted just before, it already have the leader lock.")
					return
				}
			}
		} else if !errors.IsNotFound(err) {
			klog.Error(err)
		} else {
			pod, err := client.CoreV1().Pods(podNamespace).Get(
				context.TODO(), podName, metav1.GetOptions{})
			if err != nil {
				klog.Error(err)
			} else {
				podGvk := corev1.SchemeGroupVersion.WithKind("Pod")
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      lockName,
						Namespace: podNamespace,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: podGvk.GroupVersion().String(),
								Kind:       podGvk.Kind,
								Name:       pod.GetName(),
								UID:        pod.GetUID(),
							},
						},
					},
				}
				klog.Info("try to acquire leader lock.")
				if _, err := client.CoreV1().ConfigMaps(podNamespace).Create(
					context.TODO(), configMap, metav1.CreateOptions{}); err != nil {
					if errors.IsAlreadyExists(err) {
						klog.Info("someone has already acquired the leader lock.")
					} else {
						klog.Error(err)
					}
				} else {
					klog.Info("succeed to acquire leader lock.")
					return
				}
			}
		}

		select {
		case <-time.After(wait.Jitter(defaultLeaderElectionRetryAfter, defaultLeaderElectionRetryJitterFactor)):
		case <-stopCh:
			return
		}
	}
}

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
}

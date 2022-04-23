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
	"errors"
	"fmt"
	"net/http"
	"time"

	"k8s.io/sample-controller/pkg/apis/samplecontroller/v1alpha1"

	"k8s.io/sample-controller/pkg/privatecloud"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	clientset "k8s.io/sample-controller/pkg/generated/clientset/versioned"
	samplescheme "k8s.io/sample-controller/pkg/generated/clientset/versioned/scheme"
	informers "k8s.io/sample-controller/pkg/generated/informers/externalversions/samplecontroller/v1alpha1"
	listers "k8s.io/sample-controller/pkg/generated/listers/samplecontroller/v1alpha1"
)

const controllerAgentName = "sample-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a VM is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a VM fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by VM"
	// MessageResourceSynced is the message used for an Event fired when a VM
	// is synced successfully
	MessageResourceSynced = "VM synced successfully"
)

// Controller is the controller implementation for VM resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// sampleclientset is a clientset for our own API group
	sampleclientset clientset.Interface

	vmsLister listers.VMLister
	vmsSynced cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder

	vmApi privatecloud.VmApi
}

// NewController returns a new sample controller
func NewController(
	kubeclientset kubernetes.Interface,
	sampleclientset clientset.Interface,
	vmInformer informers.VMInformer,
	vmApi privatecloud.VmApi) *Controller {

	// Create event broadcaster
	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	utilruntime.Must(samplescheme.AddToScheme(scheme.Scheme))
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:   kubeclientset,
		sampleclientset: sampleclientset,
		vmsLister:       vmInformer.Lister(),
		vmsSynced:       vmInformer.Informer().HasSynced,
		workqueue:       workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "VMs"),
		recorder:        recorder,
		vmApi:           vmApi,
	}

	klog.Info("Setting up event handlers")
	// Set up an event handler for when VM resources change
	vmInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.enqueueVM,
		UpdateFunc: controller.enqueueNewVM,
		DeleteFunc: controller.enqueueVM,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(workers int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	klog.Info("Starting VM controller")

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.vmsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	// Launch two workers to process VM resources
	for i := 0; i < workers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// VM resource to be synced.
		if err := c.syncHandler(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the VM resource
// with the current status of the resource.
func (c *Controller) syncHandler(key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the VM resource with this namespace/name
	vm, err := c.vmsLister.VMs(namespace).Get(name)
	if err != nil {
		// The VM resource may no longer exist, in which case we stop
		// processing.
		if apiErrors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("vm '%s' in work queue no longer exists", key))
			return nil
		}

		return err
	}

	if !vm.IsDeleted() {
		if vm.IsFresh() {
			if err := c.doOnCreated(vm); err != nil {
				return err
			}
		} else {
			if err := c.doOnUpdated(vm); err != nil {
				return err
			}
		}
	} else {
		if err := c.doOnDeleted(vm); err != nil {
			return err
		}
	}

	c.recorder.Event(vm, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *Controller) doOnCreated(vm *v1alpha1.VM) error {
	// call the vm creation api to private cloud.
	createVmRes, status, err := c.vmApi.Create(privatecloud.CreateVMRequest{Name: vm.Name})
	if err != nil && status != http.StatusConflict {
		return err
	}
	if createVmRes.Id == "" {
		return errors.New("there is some kind of problem in private cloud server. " +
			"the server must response not empty id in vm creation api")
	}

	// then initialize finalizers and vmId status.
	newVm := vm.DeepCopy()
	newVm.Status.VmId = createVmRes.Id
	newVm.InitializeVmFinalizer()

	updatedVm, err := c.sampleclientset.SamplecontrollerV1alpha1().VMs(vm.Namespace).UpdateStatus(
		context.TODO(), newVm, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	newVm.ResourceVersion = updatedVm.ResourceVersion
	if _, err := c.sampleclientset.SamplecontrollerV1alpha1().VMs(vm.Namespace).Update(
		context.TODO(), newVm, metav1.UpdateOptions{}); err != nil {
		return err
	}

	return nil
}

func (c *Controller) doOnUpdated(vm *v1alpha1.VM) error {
	// update cpu utilization from the vm status api.
	getStatusVmRes, _, err := c.vmApi.GetStatus(vm.Status.VmId)
	if err != nil {
		return err
	}

	newVm := vm.DeepCopy()
	newVm.Status.CpuUtilization = getStatusVmRes.CpuUtilization
	if _, err := c.sampleclientset.SamplecontrollerV1alpha1().VMs(vm.Namespace).UpdateStatus(
		context.TODO(), newVm, metav1.UpdateOptions{}); err != nil {
		return err
	}

	return nil
}

func (c *Controller) doOnDeleted(vm *v1alpha1.VM) error {
	// call the vm deletion api to private cloud.
	if status, err := c.vmApi.Delete(vm.Status.VmId); err != nil && status != http.StatusNotFound {
		return err
	}

	// then remove finalizers to remove vm completely.
	newVm := vm.DeepCopy()
	newVm.RemoveVmFinalizer()
	if _, err := c.sampleclientset.SamplecontrollerV1alpha1().VMs(vm.Namespace).Update(
		context.TODO(), newVm, metav1.UpdateOptions{}); err != nil {
		return err
	}

	return nil
}

// enqueueVM takes a VM resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than VM.
func (c *Controller) enqueueVM(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

func (c *Controller) enqueueNewVM(old interface{}, new interface{}) {
	c.enqueueVM(new)
}

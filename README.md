# sample-controller

## Running controller out of k8s cluster
```shell
$ kubectl apply -f deploy/crd.yaml
$ go build -o sample-controller
$ ./sample-controller --kubeconfig=${HOME}/.kube/config
```

## Running controller in k8s cluster

### Build controller image
```shell
$ export IMAGE=test/sample-controller:latest
$ make build
$ make push
```

### Deploy controller
```shell
$ export IMAGE=test/sample-controller:latest
$ make deploy
$ kubectl get pods
NAME                                 READY   STATUS    RESTARTS   AGE
sample-controller-59596f96b6-4tf7g   1/1     Running   0          13s
sample-controller-59596f96b6-5gdrr   1/1     Running   0          13s
sample-controller-59596f96b6-hsllm   1/1     Running   0          13s
```

### Clean controller
```shell
$ make clean
```

## Deploy VM Custom Resource

### Deploy valid VM
```shell
$ cat deploy/example/vm.yaml
apiVersion: samplecontroller.k8s.io/v1alpha1
kind: VM
metadata:
  name: vm
spec:
  vmname: vm
$ kubectl apply -f deploy/example/vm.yaml
$ kubectl get vms
NAME   VMID                                   CPUUTIL
vm     1e6abda6-807d-47aa-9edc-05eab002bc23   50
```

### Deploy invalid VM
```shell
$ cat deploy/example/vm_error.yaml
apiVersion: samplecontroller.k8s.io/v1alpha1
kind: VM
metadata:
  name: vm
spec:
  vmname: error # this vmname is not allowed by webhook
$ kubectl apply -f vm_error.yaml 
Error from server: error when applying patch:
{"metadata":{"annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"samplecontroller.k8s.io/v1alpha1\",\"kind\":\"VM\",\"metadata\":{\"annotations\":{},\"name\":\"vm\",\"namespace\":\"default\"},\"spec\":{\"vmname\":\"error\"}}\n"}},"spec":{"vmname":"error"}}
to:
Resource: "samplecontroller.k8s.io/v1alpha1, Resource=vms", GroupVersionKind: "samplecontroller.k8s.io/v1alpha1, Kind=VM"
Name: "vm", Namespace: "default"
for: "vm_error.yaml": admission webhook "samplecontroller.k8s.io" denied the request: "error" vmname is not allowed.
```

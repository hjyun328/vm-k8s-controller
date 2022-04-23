package v1alpha1

import "k8s.io/utils/strings/slices"

const finalizerName = "vm"

func (vm *VM) IsDeleted() bool {
	return !vm.DeletionTimestamp.IsZero()
}

func (vm *VM) IsFresh() bool {
	return !vm.IsDeleted() && !vm.hasVmFinalizer()
}

func (vm *VM) InitializeVmFinalizer() {
	if !vm.hasVmFinalizer() {
		vm.ObjectMeta.Finalizers = append(vm.ObjectMeta.Finalizers, finalizerName)
	}
}

func (vm *VM) RemoveVmFinalizer() {
	vm.ObjectMeta.Finalizers = slices.Filter([]string{}, vm.ObjectMeta.Finalizers, func(f string) bool {
		return f != finalizerName
	})
}

func (vm *VM) hasVmFinalizer() bool {
	return slices.Contains(vm.ObjectMeta.Finalizers, finalizerName)
}

package internal

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"k8s.io/klog/v2"
	"k8s.io/sample-controller/pkg/apis/samplecontroller/v1alpha1"
	"k8s.io/sample-controller/pkg/privatecloud"

	betav1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VMValidatingWebHook struct {
	vmApi privatecloud.VmApi
}

func (v *VMValidatingWebHook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var arReqBytes []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			arReqBytes = data
		} else {
			klog.Errorf("cannot read AdmissionReview request body: %v\n", err)
			http.Error(w, "cannot read AdmissionReview request body", http.StatusInternalServerError)
			return
		}
	}
	if r.Header.Get("Content-Type") != "application/json" {
		klog.Infof("only support application/json content type\n")
		http.Error(w, "only support application/json content type", http.StatusBadRequest)
		return
	}
	if len(arReqBytes) == 0 {
		klog.Infof("empty AdmissionReview request body\n")
		http.Error(w, "empty AdmissionReview request body", http.StatusBadRequest)
		return
	}

	arReq := betav1.AdmissionReview{}
	if err := json.Unmarshal(arReqBytes, &arReq); err != nil {
		klog.Infof("cannot unmarshal AdmissionReview: %v\n", err)
		http.Error(w, "cannot unmarshal AdmissionReview", http.StatusBadRequest)
		return
	}

	vm := v1alpha1.VM{}
	raw := arReq.Request.Object.Raw
	if err := json.Unmarshal(raw, &vm); err != nil {
		klog.Infof("cannot unmarshal Raw of AdmissionReview: %v\n", err)
		http.Error(w, "cannot unmarshal Raw of AdmissionReview", http.StatusBadRequest)
		return
	}

	adrAllowed := true
	adrMessage := "vmname is valid."
	if vm.Spec.VMName == "" {
		adrAllowed = false
		adrMessage = "vmname is empty."
	} else {
		_, err := v.vmApi.Check(vm.Spec.VMName)
		if err != nil {
			adrAllowed = false
			adrMessage = err.Error()
		}
	}

	arResBytes, err := json.Marshal(betav1.AdmissionReview{
		Response: &betav1.AdmissionResponse{
			Allowed: adrAllowed,
			Result: &metav1.Status{
				Message: adrMessage,
			},
		},
	})
	if err != nil {
		klog.Errorf("cannot marshal AdmissionResponse: %v\n", err)
		http.Error(w, "cannot marshal AdmissionResponse", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(arResBytes); err != nil {
		klog.Errorf("cannot write AdmissionResponse: %v\n", err)
		http.Error(w, "cannot write response", http.StatusInternalServerError)
		return
	}
}

func NewVMValidatingWebHook(vmApi privatecloud.VmApi) *VMValidatingWebHook {
	return &VMValidatingWebHook{vmApi: vmApi}
}

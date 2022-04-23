package privatecloud

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/pkg/errors"
)

type VmApi interface {
	Create(CreateVMRequest) (CreateVMResponse, int, error)
	GetStatus(uuid string) (GetStatusVMResponse, int, error)
	Delete(uuid string) (int, error)
	Check(name string) (int, error)
}

type vmApi struct {
	hostPort    string
	httpTimeout time.Duration
}

func (v *vmApi) createHttpClient() http.Client {
	return http.Client{Timeout: v.httpTimeout}
}

func (v *vmApi) Create(creq CreateVMRequest) (cres CreateVMResponse, status int, err error) {
	reqJsonBytes, err := json.Marshal(creq)
	if err != nil {
		return cres, 0, err
	}
	reqJsonBuffer := bytes.NewBuffer(reqJsonBytes)
	httpClient := v.createHttpClient()

	req, err := http.NewRequest(http.MethodPost,
		"http://"+v.hostPort+"/servers", reqJsonBuffer)
	if err != nil {
		return cres, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := httpClient.Do(req)
	if err != nil {
		return cres, 0, err
	}

	defer res.Body.Close()

	resBodyBytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return cres, res.StatusCode, err
	}

	if res.StatusCode >= 200 && res.StatusCode < 300 {
	} else {
		return cres, res.StatusCode, errors.New(string(resBodyBytes))
	}

	switch sc := res.StatusCode; {
	case sc >= 200 && sc < 300:
	default:
		return cres, res.StatusCode, errors.New(string(resBodyBytes))
	}

	if err := json.Unmarshal(resBodyBytes, &cres); err != nil {
		return cres, 0, err
	}

	return cres, res.StatusCode, nil
}

func (v *vmApi) GetStatus(uuid string) (gres GetStatusVMResponse, status int, err error) {
	httpClient := v.createHttpClient()

	req, err := http.NewRequest(http.MethodGet,
		"http://"+v.hostPort+"/servers/"+uuid+"/status", nil)
	if err != nil {
		return gres, 0, nil
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return gres, 0, err
	}
	defer res.Body.Close()

	resBodyBytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return gres, res.StatusCode, err
	}

	switch sc := res.StatusCode; {
	case sc >= 200 && sc < 300:
	default:
		return gres, res.StatusCode, errors.New(string(resBodyBytes))
	}

	if err := json.Unmarshal(resBodyBytes, &gres); err != nil {
		return gres, res.StatusCode, err
	}

	return gres, res.StatusCode, nil
}

func (v *vmApi) Delete(uuid string) (status int, err error) {
	httpClient := v.createHttpClient()

	req, err := http.NewRequest(http.MethodDelete,
		"http://"+v.hostPort+"/servers/"+uuid, nil)
	if err != nil {
		return 0, err
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	resBodyBytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return res.StatusCode, err
	}

	switch sc := res.StatusCode; {
	case sc >= 200 && sc < 300:
	default:
		return res.StatusCode, errors.New(string(resBodyBytes))
	}

	return res.StatusCode, nil
}

func (v *vmApi) Check(name string) (status int, err error) {
	httpClient := v.createHttpClient()

	req, err := http.NewRequest(http.MethodGet,
		"http://"+v.hostPort+"/check/"+name, nil)
	if err != nil {
		return 0, err
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	resBodyBytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return res.StatusCode, err
	}

	switch sc := res.StatusCode; {
	case sc >= 200 && sc < 300:
	default:
		return res.StatusCode, errors.New(string(resBodyBytes))
	}

	return res.StatusCode, nil
}

func DefaultVmApi(hostPort string, httpTimeout time.Duration) VmApi {
	return &vmApi{hostPort: hostPort, httpTimeout: httpTimeout}
}

type NoopVmApi struct {
}

func (n *NoopVmApi) Create(CreateVMRequest) (CreateVMResponse, int, error) {
	return CreateVMResponse{Id: string(uuid.NewUUID())}, http.StatusCreated, nil
}

func (n *NoopVmApi) GetStatus(uuid string) (GetStatusVMResponse, int, error) {
	return GetStatusVMResponse{CpuUtilization: 50}, http.StatusOK, nil
}

func (n *NoopVmApi) Delete(uuid string) (int, error) {
	return http.StatusNoContent, nil
}

func (n *NoopVmApi) Check(name string) (int, error) {
	if name == "error" {
		return http.StatusBadRequest, errors.New(`"error" vmname is not allowed.`)
	}
	return http.StatusOK, nil
}

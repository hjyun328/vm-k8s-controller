package privatecloud

type CreateVMRequest struct {
	Name string `json:"name"`
}

type CreateVMResponse struct {
	Id string `json:"id"`
}

type GetStatusVMResponse struct {
	CpuUtilization uint `json:"cpuUtilization"`
}

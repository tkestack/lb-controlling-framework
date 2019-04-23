package backendgroup

type BackendType string

const(
	TypeService BackendType = "service"
	TypePod BackendType = "pod"
	TypeStatic BackendType = "static"
)

type BackendGroupInternal struct {
	Name    string
	Service *ServiceBackend
	Pods    *PodBackend
	Static  *StaticBackend
}

type ServiceBackend struct {
	Name         string
	Port         PortSelector
	NodeSelector map[string]string
}

type PodBackend struct {
	Port    PortSelector
	ByLabel *SelectPodByLabel
	ByName  []string
}

type StaticBackend struct {
	Backends []string
}

type PortSelector struct {
	PortNumber int32
	Protocol   *string
}

type SelectPodByLabel struct {
	Selector map[string]string
	Except   []string
}


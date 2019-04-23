package loadbalancer

type LoadBalancerInternal struct {
	Name string
	LBDriver string
	LBSpec map[string]string
	Attributes map[string]string
}

package loadbalancer

import (
	"reflect"

	lbcf "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/apis/lbcf.tke.cloud.tencent.com/v1beta1"
)

func LBUpdatedFieldsAllowed(cur *lbcf.LoadBalancer, old *lbcf.LoadBalancer) bool {
	if cur.Name != old.Name {
		return false
	}
	if cur.Spec.LBDriver!= old.Spec.LBDriver{
		return false
	}
	if !reflect.DeepEqual(cur.Spec.LBSpec, old.Spec.LBSpec) {
		return false
	}
	return true
}

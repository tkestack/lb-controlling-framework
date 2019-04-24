package lbcfcontroller

import (
	lbcfapi "git.tencent.com/tke/lb-controlling-framework/pkg/apis/lbcf.tke.cloud.tencent.com/v1beta1"
	lbcfclient "git.tencent.com/tke/lb-controlling-framework/pkg/client-go/clientset/versioned"
	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/listers/lbcf.tke.cloud.tencent.com/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/tools/cache"
	"time"
)

type BackendProvider interface {
	listBackendByDriver(driverName string, driverNamespace string) ([]*lbcfapi.BackendRecord, error)
}

func NewBackendController(client *lbcfclient.Clientset, bgLister v1beta1.BackendGroupLister, brLister v1beta1.BackendRecordLister, driverProvider DriverProvider, lbProvider LoadBalancerProvider) *BackendController {
	return &BackendController{
		client:         client,
		bgLister:       bgLister,
		brLister:       brLister,
		driverProvider: driverProvider,
		lbProvider:     lbProvider,
	}
}

type BackendController struct {
	client         *lbcfclient.Clientset

	bgLister       v1beta1.BackendGroupLister
	brLister       v1beta1.BackendRecordLister

	driverProvider DriverProvider
	lbProvider     LoadBalancerProvider
}

func (c *BackendController) syncBackendGroup(key string) (error, *time.Duration) {

	return nil, nil
}

func (c *BackendController) syncBackend(key string) (error, *time.Duration){
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil{
		return err, nil
	}
	backend, err := c.brLister.BackendRecords(namespace).Get(name)
	if errors.IsNotFound(err){
		return nil, nil
	}else if err != nil{
		return err, nil
	}

	driver, err := c.driverProvider.getDriver(backend.Spec.LBDriver, getDriverNamespace(backend.Spec.LBDriver, backend.Namespace))
	if err != nil {
		return err, nil
	}

	// TODO: deregister backend
	if backend.DeletionTimestamp != nil{
		// TODO: handle finalizer

		// TODO: handle backendRecord with backendAddr

		// handle backendRecord with no backendAddr
		if backend.Status.BackendAddr == ""{
			return nil, nil
		}
		return nil, nil
	}

	// TODO: generateBackendAddr
	if backend.Status.BackendAddr == ""{
		req := &GenerateBackendAddrRequest{
			RequestForRetryHooks: RequestForRetryHooks{
				RecordID: string(backend.UID),
				RetryID:  string(uuid.NewUUID()),
			},
			// TODO: service backend?
		}
		callGenerateBackendAddr(driver, req)
		return nil, nil
	}

	// TODO: ensureBackend

	return nil, nil
}

func (c *BackendController) listBackendByDriver(driverName string, driverNamespace string) ([]*lbcfapi.BackendRecord, error) {
	recordList, err := c.brLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	var ret []*lbcfapi.BackendRecord
	for _, r := range recordList {
		if driverNamespace != "kube-system" && r.Namespace != driverNamespace {
			continue
		}
		if r.Spec.LBDriver == driverName {
			ret = append(ret, r)
		}
	}
	return ret, nil
}

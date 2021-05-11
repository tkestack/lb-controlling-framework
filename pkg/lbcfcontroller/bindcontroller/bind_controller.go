package bindcontroller

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	bindutil "tkestack.io/lb-controlling-framework/pkg/api/bind"
	lbcfv1 "tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1"
	"tkestack.io/lb-controlling-framework/pkg/apis/lbcf.tkestack.io/v1beta1"
	lbcfclient "tkestack.io/lb-controlling-framework/pkg/client-go/clientset/versioned"
	v1 "tkestack.io/lb-controlling-framework/pkg/client-go/listers/lbcf.tkestack.io/v1"
	lbcflister "tkestack.io/lb-controlling-framework/pkg/client-go/listers/lbcf.tkestack.io/v1beta1"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/util"
	"tkestack.io/lb-controlling-framework/pkg/lbcfcontroller/webhooks"
	"tkestack.io/lb-controlling-framework/pkg/metrics"

	apicorev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	corev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
)

func NewController(
	client lbcfclient.Interface,
	driverLister lbcflister.LoadBalancerDriverLister,
	bindLister v1.BindLister,
	brLister lbcflister.BackendRecordLister,
	podLister corev1.PodLister,
	invoker util.WebhookInvoker,
	recorder record.EventRecorder,
	dryRun bool,
) *Controller {
	return &Controller{
		client:         client,
		driverLister:   driverLister,
		bindLister:     bindLister,
		brLister:       brLister,
		podLister:      podLister,
		webhookInvoker: invoker,
		eventRecorder:  recorder,
		dryRun:         dryRun,
	}
}

type Controller struct {
	client lbcfclient.Interface

	driverLister lbcflister.LoadBalancerDriverLister
	bindLister   v1.BindLister
	brLister     lbcflister.BackendRecordLister
	podLister    corev1.PodLister

	webhookInvoker util.WebhookInvoker
	eventRecorder  record.EventRecorder
	dryRun         bool
}

func (c *Controller) Sync(key string) *util.SyncResult {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return util.ErrorResult(err)
	}
	bind, err := c.bindLister.Binds(namespace).Get(name)
	if errors.IsNotFound(err) {
		klog.Infof("Bind %s/%s not found, finished", namespace, name)
		return util.FinishedResult()
	} else if err != nil {
		return util.ErrorResult(err)
	}
	if bind.DeletionTimestamp != nil {
		if !util.HasFinalizer(bind.Finalizers, lbcfv1.FinalizerDeleteLB) {
			return util.FinishedResult()
		}
		existBackendRecords, err := c.brLister.BackendRecords(bind.Namespace).List(labels.SelectorFromSet(map[string]string{
			lbcfv1.LabelBindName: bind.Name,
		}))
		if err != nil {
			klog.Errorf("list BackendRecords for Bind %s/%s failed, can't delete bind",
				bind.Namespace, bind.Name)
			c.eventRecorder.Eventf(
				bind,
				apicorev1.EventTypeWarning,
				"ListBackendRecordsFailed",
				err.Error())
			return util.AsyncResult(10 * time.Second)
		}
		if len(bind.Status.LoadBalancerStatuses) == 0 && len(existBackendRecords) == 0 {
			if err := c.removeFinalizer(bind); err != nil {
				return util.ErrorResult(err)
			}
			return util.FinishedResult()
		}
	}
	needResync := c.handleLoadBalancer(bind)
	needResync = needResync || c.handleBackends(bind)
	if needResync {
		return util.AsyncResult(10 * time.Second)
	}
	return util.FinishedResult()
}

func (c *Controller) ListRelatedBindForPod(pod *apicorev1.Pod) sets.String {
	bindList, err := c.bindLister.Binds(pod.Namespace).List(labels.Everything())
	if err != nil {
		klog.Errorf("list related Bind for pod %s/%s failed: %v", pod.Namespace, pod.Name, err)
		return nil
	}
	ret := sets.NewString()
	for _, bind := range bindList {
		var key string
		if len(bind.Namespace) > 0 {
			key = bind.Namespace + "/" + bind.Name
		} else {
			key = bind.Name
		}
		if bind.Spec.Pods.ByLabel != nil {
			except := sets.NewString(bind.Spec.Pods.ByLabel.Except...)
			if except.Has(pod.Name) {
				continue
			}
			selector := labels.SelectorFromSet(bind.Spec.Pods.ByLabel.Selector)
			if selector.Matches(labels.Set(pod.Labels)) {
				ret.Insert(key)
			}
		} else {
			expect := sets.NewString(bind.Spec.Pods.ByName...)
			if expect.Has(pod.Name) {
				ret.Insert(key)
			}
		}
	}
	return ret
}

func (c *Controller) handleLoadBalancer(bind *lbcfv1.Bind) (needResync bool) {
	statusMap := make(map[string]lbcfv1.TargetLoadBalancerStatus)
	for _, s := range bind.Status.LoadBalancerStatuses {
		statusMap[s.Name] = s
	}
	var lbNeedCreate, lbNeedEnsure []lbcfv1.TargetLoadBalancer
	var lbNeedDelete []lbcfv1.TargetLoadBalancerStatus
	if bind.DeletionTimestamp != nil {
		for _, status := range bind.Status.LoadBalancerStatuses {
			if len(status.LBInfo) > 0 {
				lbNeedDelete = append(lbNeedDelete, status)
			}
		}
	} else {
		for _, lb := range bind.Spec.LoadBalancers {
			if curStatus, ok := statusMap[lb.Name]; !ok || !bindutil.IsLoadBalancerCreated(curStatus) {
				lbNeedCreate = append(lbNeedCreate, lb)
			} else if !util.EqualStringMap(lb.Attributes, curStatus.LastSyncedAttributes) {
				lbNeedEnsure = append(lbNeedEnsure, lb)
			}
		}
		for _, status := range bind.Status.LoadBalancerStatuses {
			if status.DeletionTimestamp != nil {
				lbNeedDelete = append(lbNeedDelete, status)
			}
		}
		if len(lbNeedCreate)+len(lbNeedEnsure)+len(lbNeedDelete) == 0 {
			klog.Infof("skip handling load balancers of Bind %s/%s", bind.Namespace, bind.Name)
			return false
		}
	}

	// start load balancer operations
	klog.Infof("start handling load balancers of Bind %s/%s", bind.Namespace, bind.Name)
	wg := sync.WaitGroup{}
	result := new(sync.Map)
	for _, lb := range lbNeedCreate {
		driver := c.getDriver(bind, lb.Name, lb.Driver)
		if driver == nil {
			needResync = true
			continue
		}
		wg.Add(1)
		go func(lb lbcfv1.TargetLoadBalancer, driver *v1beta1.LoadBalancerDriver) {
			defer wg.Done()
			rsp, err := c.createLB(bind, lb, driver)
			op := &lbOperation{
				lbName:       lb.Name,
				lbDriver:     lb.Driver,
				lbSpec:       lb.Spec,
				lbAttributes: lb.Attributes,
				opType:       operationCreate,
				err:          err,
				createRsp:    rsp,
			}
			result.Store(lb.Name, op)
		}(lb, driver)
	}
	for _, lb := range lbNeedEnsure {
		driver := c.getDriver(bind, lb.Name, lb.Driver)
		if driver == nil {
			needResync = true
			continue
		}
		wg.Add(1)
		go func(lb lbcfv1.TargetLoadBalancer, driver *v1beta1.LoadBalancerDriver, curStatus lbcfv1.TargetLoadBalancerStatus) {
			defer wg.Done()
			rsp, err := c.ensureLB(bind, lb, driver)
			op := &lbOperation{
				lbName:                lb.Name,
				lbDriver:              lb.Driver,
				lbSpec:                lb.Spec,
				lbAttributes:          lb.Attributes,
				statusBeforeOperation: curStatus,
				opType:                operationEnsure,
				err:                   err,
				ensureRsp:             rsp,
			}
			result.Store(lb.Name, op)
		}(lb, driver, statusMap[lb.Name])
	}
	for _, status := range lbNeedDelete {
		driver := c.getDriver(bind, status.Name, status.Driver)
		if driver == nil {
			needResync = true
			continue
		}
		wg.Add(1)
		go func(status lbcfv1.TargetLoadBalancerStatus, driver *v1beta1.LoadBalancerDriver) {
			defer wg.Done()
			rsp, err := c.deleteLB(bind, status, driver)
			op := &lbOperation{
				lbName:                status.Name,
				lbDriver:              status.Driver,
				lbSpec:                status.LBInfo,
				lbAttributes:          status.LastSyncedAttributes,
				statusBeforeOperation: status,
				opType:                operationDelete,
				err:                   err,
				deleteRsp:             rsp,
			}
			result.Store(status.Name, op)
		}(status, driver)
	}
	wg.Wait()

	if c.dryRun {
		return false
	}

	// parse results
	var newStatuses, needDeleteStatuses []lbcfv1.TargetLoadBalancerStatus
	result.Range(func(key, value interface{}) bool {
		op := value.(*lbOperation)
		switch op.opType {
		case operationCreate:
			sts, reCheck := op.parseCreateResult()
			if sts != nil {
				newStatuses = append(newStatuses, *sts)
			}
			if reCheck {
				needResync = reCheck
			}
		case operationEnsure:
			sts, reCheck := op.parseEnsureResult()
			if sts != nil {
				newStatuses = append(newStatuses, *sts)
			}
			if reCheck {
				needResync = reCheck
			}
		case operationDelete:
			newSts, needDeleteSts, reCheck := op.parseDeleteResult()
			if newSts != nil {
				newStatuses = append(newStatuses, *newSts)
			} else if needDeleteSts != nil {
				needDeleteStatuses = append(needDeleteStatuses, *needDeleteSts)
			}
			if reCheck {
				needResync = reCheck
			}
		}
		return true
	})

	// update bind status
	mergedStatus := mergeStatus(bind.Status.LoadBalancerStatuses, newStatuses, needDeleteStatuses)
	cpy := bind.DeepCopy()
	cpy.Status.LoadBalancerStatuses = mergedStatus
	if _, err := c.client.LbcfV1().Binds(bind.Namespace).UpdateStatus(cpy); err != nil {
		klog.Errorf("update status of Bind %s/%s failed: %v", bind.Namespace, bind.Name, err)
		return true
	}
	return
}

func (c *Controller) handleBackends(bind *lbcfv1.Bind) (needResync bool) {
	// delete all BackendRecordss by label
	if bind.DeletionTimestamp != nil {
		c.eventRecorder.Eventf(
			bind,
			apicorev1.EventTypeNormal,
			"DeleteAllBackends",
			"deleting all BackendRecords")

		selector := labels.SelectorFromSet(map[string]string{
			lbcfv1.LabelBindName: bind.Name,
		})
		listOption := metav1.ListOptions{
			LabelSelector: selector.String(),
		}
		if err := c.client.LbcfV1beta1().BackendRecords(bind.Namespace).DeleteCollection(&metav1.DeleteOptions{}, listOption); err != nil {
			klog.Errorf("deleteCollection for Bind %s/%s failed: %v",
				bind.Namespace, bind.Name, err)
			c.eventRecorder.Eventf(
				bind,
				apicorev1.EventTypeWarning,
				"DeleteCollectionFailed",
				err.Error())
		}
		return true
	}

	var expected []*v1beta1.BackendRecord
	doNotDelete := sets.NewString()
	var err error
	expected, doNotDelete, err = c.expectedPodBackends(bind)
	if err != nil {
		klog.Errorf("handle Bind %s/%s failed: %v", bind.Namespace, bind.Name, err)
		c.eventRecorder.Eventf(bind, apicorev1.EventTypeWarning, "HandleBackendFailed", err.Error())
		return true
	}
	existBackendRecords, err := c.brLister.BackendRecords(bind.Namespace).List(labels.SelectorFromSet(map[string]string{
		lbcfv1.LabelBindName: bind.Name,
	}))
	if err != nil {
		klog.Errorf("list BackendRecords for Bind %s/%s failed: %v", bind.Namespace, bind.Name, err)
		c.eventRecorder.Eventf(
			bind,
			apicorev1.EventTypeWarning,
			"ListBackendFailed",
			err.Error())
		return true
	}
	needCreate, needUpdate, needDelete := compareBackendRecords(expected, existBackendRecords, doNotDelete)
	var errs util.ErrorList
	if err := util.IterateBackends(needDelete, c.deleteBackendRecord); err != nil {
		klog.Errorf("delete BackendRecords for Bind %s/%s failed: %v",
			bind.Namespace, bind.Name, err)
		c.eventRecorder.Eventf(bind, apicorev1.EventTypeWarning, "DeleteBackendFailed", err.Error())
		errs = append(errs, err)
	}
	if err := util.IterateBackends(needUpdate, c.updateBackendRecord); err != nil {
		klog.Errorf("update BackendRecords for Bind %s/%s failed: %v",
			bind.Namespace, bind.Name, err)
		c.eventRecorder.Eventf(bind, apicorev1.EventTypeWarning, "UpdateBackendFailed", err.Error())
		errs = append(errs, err)
	}
	if err := util.IterateBackends(needCreate, c.createBackendRecord); err != nil {
		klog.Errorf("create BackendRecords for Bind %s/%s failed: %v",
			bind.Namespace, bind.Name, err)
		c.eventRecorder.Eventf(bind, apicorev1.EventTypeWarning, "CreateBackendFailed", err.Error())
		errs = append(errs, err)
	}
	return len(errs) > 0
}

func (c *Controller) expectedPodBackends(bind *lbcfv1.Bind) (expected []*v1beta1.BackendRecord, doNotDelete sets.String, err error) {
	var createdLB []lbcfv1.TargetLoadBalancer
	statusMap := make(map[string]lbcfv1.TargetLoadBalancerStatus)
	for _, status := range bind.Status.LoadBalancerStatuses {
		statusMap[status.Name] = status
	}
	for _, lb := range bind.Spec.LoadBalancers {
		sts, ok := statusMap[lb.Name]
		if !ok {
			continue
		}
		if bindutil.IsLoadBalancerCreated(sts) && sts.DeletionTimestamp == nil {
			createdLB = append(createdLB, lb)
		}
	}

	var pods []*apicorev1.Pod
	if bind.Spec.Pods.ByLabel != nil {
		podList, err := c.podLister.Pods(bind.Namespace).List(labels.SelectorFromSet(bind.Spec.Pods.ByLabel.Selector))
		if err != nil {
			klog.Errorf("list pods for Bind %s/%s failed: %v", bind.Namespace, bind.Name, err)
			c.eventRecorder.Eventf(
				bind,
				apicorev1.EventTypeWarning,
				"ListPodFailed",
				fmt.Sprintf("list pods failed: %v", err))
			return nil, nil, err
		}
		except := sets.NewString(bind.Spec.Pods.ByLabel.Except...)
		for _, pod := range podList {
			if _, ok := except[pod.Name]; !ok {
				pods = append(pods, pod)
			}
		}
	} else {
		for _, expect := range bind.Spec.Pods.ByName {
			pod, err := c.podLister.Pods(bind.Namespace).Get(expect)
			if err != nil {
				if errors.IsNotFound(err) {
					klog.Infof("pod %s/%s selected by Bind %s/%s doesn't exist, skipped",
						bind.Namespace, expect, bind.Namespace, bind.Namespace)
				} else {
					klog.Errorf("get pod %s/%s selected by Bind %s/%s failed: %v",
						bind.Namespace, expect, bind.Namespace, bind.Namespace, err)
					c.eventRecorder.Eventf(
						bind,
						apicorev1.EventTypeWarning,
						"GetPodFailed",
						fmt.Sprintf("get pod %s/%s failed: %v", bind.Namespace, expect, err))
				}
				continue
			}
			pods = append(pods, pod)
		}
	}
	var readyPods, podsDoNotDelete []*apicorev1.Pod
	for _, pod := range pods {
		if util.PodAvailable(pod) {
			readyPods = append(readyPods, pod)
		} else if bindutil.DeregIfNotRunning(bind) {
			if util.PodAvailableByRunning(pod) {
				podsDoNotDelete = append(podsDoNotDelete, pod)
			}
		} else if bindutil.DeregByWebhook(bind) {
			// todo
		}
	}

	doNotDelete = sets.NewString()
	for _, lb := range createdLB {
		lbStatus := statusMap[lb.Name]
		for _, pod := range readyPods {
			expected = append(expected, generateBackendRecord(bind, lb, lbStatus, pod)...)
		}
		for _, pod := range podsDoNotDelete {
			for _, port := range bind.Spec.Pods.Ports {
				brName := getBRName(
					bind.Name,
					lb.Name,
					pod.Name,
					pod.Status.PodIP,
					port.Protocol,
					fmt.Sprintf("%d", port.Port))
				doNotDelete.Insert(brName)
			}
		}
	}
	if klog.V(3) {
		var info []string
		for _, e := range expected {
			info = append(info, fmt.Sprintf("\t[pod]%s/%s:%s/%d --> [lb]%s\n",
				e.Namespace, e.Spec.PodBackendInfo.Name, e.Spec.PodBackendInfo.Port.Protocol, e.Spec.PodBackendInfo.Port.Port,
				e.Labels[lbcfv1.LabelLBName]))
		}
		klog.Infof("Bind %s/%s expecting following BackendRecord:\n%s", bind.Namespace, bind.Name, strings.Join(info, ""))
	}
	return expected, doNotDelete, nil

}

func (c *Controller) createBackendRecord(record *v1beta1.BackendRecord) error {
	// in dry-run mode, BackendRecord is printed without being created
	if c.dryRun {
		var extraInfo string
		if klog.V(3) {
			b, _ := json.Marshal(record)
			extraInfo = fmt.Sprintf("created BackendRecord: %s", string(b))
		}
		klog.Infof("[dry-run] create BackendRecord %s/%s. %s", record.Namespace, record.Name, extraInfo)
		return nil
	}

	start := time.Now()
	_, err := c.client.LbcfV1beta1().BackendRecords(record.Namespace).Create(record)
	metrics.K8SOpLatencyObserve("BackendRecord", metrics.OpCreate, time.Since(start))
	if err != nil {
		return fmt.Errorf("create BackendRecord %s/%s failed: %v", record.Namespace, record.Name, err)
	}
	return nil
}

func (c *Controller) updateBackendRecord(record *v1beta1.BackendRecord) error {
	// in dry-run mode, BackendRecord is printed without being updated
	if c.dryRun {
		var extraInfo string
		if klog.V(3) {
			b, _ := json.Marshal(record)
			extraInfo = fmt.Sprintf("updated BackendRecord: %s", string(b))
		}
		klog.Infof("[dry-run] update BackendRecord %s/%s. %s", record.Namespace, record.Name, extraInfo)
		return nil
	}

	start := time.Now()
	_, err := c.client.LbcfV1beta1().BackendRecords(record.Namespace).Update(record)
	metrics.K8SOpLatencyObserve("BackendRecord", metrics.OpUpdate, time.Since(start))
	if err != nil {
		return fmt.Errorf("update BackendRecord %s/%s failed: %v", record.Namespace, record.Name, err)
	}
	return nil
}

func (c *Controller) deleteBackendRecord(record *v1beta1.BackendRecord) error {
	if record.DeletionTimestamp != nil {
		return nil
	}
	// in dry-run mode, BackendRecord is printed without being deleted
	if c.dryRun {
		klog.Infof("[dry-run] delete BackendRecord %s/%s", record.Namespace, record.Name)
		return nil
	}

	start := time.Now()
	err := c.client.LbcfV1beta1().BackendRecords(record.Namespace).Delete(record.Name, nil)
	metrics.K8SOpLatencyObserve("BackendRecord", metrics.OpDelete, time.Since(start))
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("delete BackendRecord %s/%s failed: %v", record.Namespace, record.Name, err)
	}
	return nil
}

func (c *Controller) getDriver(bind *lbcfv1.Bind, lbName, driverName string) *v1beta1.LoadBalancerDriver {
	driver, err := c.driverLister.LoadBalancerDrivers(util.NamespaceOfSharedObj(driverName, bind.Namespace)).Get(driverName)
	if err != nil {
		klog.Errorf("get driver %q for LoadBalancer %s in Bind %s/%s failed: %v",
			driverName, lbName, bind.Namespace, bind.Name, err)
		c.eventRecorder.Eventf(
			bind,
			apicorev1.EventTypeWarning,
			"DriverNotFound",
			fmt.Sprintf("driver %s for load balancer %s not found", driverName, lbName))
		return nil
	}
	return driver
}

func (c *Controller) createLB(
	bind *lbcfv1.Bind,
	lb lbcfv1.TargetLoadBalancer,
	driver *v1beta1.LoadBalancerDriver) (*webhooks.CreateLoadBalancerResponse, error) {
	req := &webhooks.CreateLoadBalancerRequest{
		RequestForRetryHooks: webhooks.RequestForRetryHooks{
			RecordID: fmt.Sprintf("createLoadBalancer(%s-%s)", bind.UID, lb.Name),
			RetryID:  string(uuid.NewUUID()),
		},
		LBSpec:     lb.Spec,
		Attributes: lb.Attributes,
		DryRun:     c.dryRun,
	}
	if c.dryRun {
		klog.Infof("[dry-run] webhook: createLoadBalancer, Bind: %s/%s, LoadBalancer: %s",
			bind.Namespace, bind.Name, lb.Name)
		if !driver.Spec.AcceptDryRunCall {
			return &webhooks.CreateLoadBalancerResponse{
				ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
					Status: webhooks.StatusSucc,
				},
			}, nil
		}
	}
	return c.webhookInvoker.CallCreateLoadBalancer(driver, req)
}

func (c *Controller) ensureLB(
	bind *lbcfv1.Bind,
	lb lbcfv1.TargetLoadBalancer,
	driver *v1beta1.LoadBalancerDriver,
) (*webhooks.EnsureLoadBalancerResponse, error) {
	req := &webhooks.EnsureLoadBalancerRequest{
		RequestForRetryHooks: webhooks.RequestForRetryHooks{
			RecordID: fmt.Sprintf("ensureLoadBalancer(%s)", bind.UID),
			RetryID:  string(uuid.NewUUID()),
		},
		Attributes: lb.Attributes,
		DryRun:     c.dryRun,
	}
	if c.dryRun {
		klog.Infof("[dry-run] webhook: ensureLoadBalancer, Bind: %s/%s, LoadBalancer: %s",
			bind.Namespace, bind.Name, lb.Name)
		if !driver.Spec.AcceptDryRunCall {
			return &webhooks.EnsureLoadBalancerResponse{
				ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
					Status: webhooks.StatusSucc,
				},
			}, nil
		}
	}
	return c.webhookInvoker.CallEnsureLoadBalancer(driver, req)
}

func (c *Controller) deleteLB(
	bind *lbcfv1.Bind,
	lbStatus lbcfv1.TargetLoadBalancerStatus,
	driver *v1beta1.LoadBalancerDriver,
) (*webhooks.DeleteLoadBalancerResponse, error) {
	req := &webhooks.DeleteLoadBalancerRequest{
		RequestForRetryHooks: webhooks.RequestForRetryHooks{
			RecordID: fmt.Sprintf("deleteLoadBalancer(%s)", bind.UID),
			RetryID:  string(uuid.NewUUID()),
		},
		Attributes: lbStatus.LastSyncedAttributes,
		DryRun:     c.dryRun,
	}
	if c.dryRun {
		klog.Infof("[dry-run] webhook: deleteLoadBalancer, Bind: %s/%s, LoadBalancer: %s",
			bind.Namespace, bind.Name, lbStatus.Name)
		if !driver.Spec.AcceptDryRunCall {
			return &webhooks.DeleteLoadBalancerResponse{
				ResponseForFailRetryHooks: webhooks.ResponseForFailRetryHooks{
					Status: webhooks.StatusSucc,
				},
			}, nil
		}
	}
	return c.webhookInvoker.CallDeleteLoadBalancer(driver, req)
}

func (c *Controller) removeFinalizer(bind *lbcfv1.Bind) error {
	cpy := bind.DeepCopy()
	cpy.Finalizers = util.RemoveFinalizer(cpy.Finalizers, lbcfv1.FinalizerDeleteLB)
	_, err := c.client.LbcfV1().Binds(cpy.Namespace).Update(cpy)
	return err
}

type operationType int

const (
	operationCreate operationType = iota
	operationEnsure
	operationDelete
)

type lbOperation struct {
	lbName                string
	lbDriver              string
	lbSpec                map[string]string
	lbAttributes          map[string]string
	statusBeforeOperation lbcfv1.TargetLoadBalancerStatus
	opType                operationType
	err                   error
	createRsp             *webhooks.CreateLoadBalancerResponse
	ensureRsp             *webhooks.EnsureLoadBalancerResponse
	deleteRsp             *webhooks.DeleteLoadBalancerResponse
}

func (op lbOperation) parseCreateResult() (*lbcfv1.TargetLoadBalancerStatus, bool) {
	if op.opType != operationCreate {
		return nil, false
	}
	if op.err != nil {
		return &lbcfv1.TargetLoadBalancerStatus{
			Name:       op.lbName,
			Driver:     op.lbDriver,
			RetryAfter: metav1.NewTime(time.Now().Add(util.CalculateRetryInterval(20))),
			Conditions: []lbcfv1.TargetLoadBalancerCondition{
				{
					Type:               lbcfv1.LBCreated,
					Status:             lbcfv1.ConditionFalse,
					LastTransitionTime: metav1.Now(),
					Reason:             lbcfv1.ReasonWebhookError,
					Message:            op.err.Error(),
				},
			},
		}, true
	}

	switch op.createRsp.Status {
	case webhooks.StatusSucc:
		lbInfo := op.lbSpec
		if len(op.createRsp.LBInfo) > 0 {
			lbInfo = op.createRsp.LBInfo
		}
		return &lbcfv1.TargetLoadBalancerStatus{
			Name:                 op.lbName,
			Driver:               op.lbDriver,
			LBInfo:               lbInfo,
			LastSyncedAttributes: op.lbAttributes,
			RetryAfter:           metav1.Time{},
			Conditions: []lbcfv1.TargetLoadBalancerCondition{
				{
					Type:               lbcfv1.LBCreated,
					Status:             lbcfv1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
				},
				{
					Type:               lbcfv1.LBReady,
					Status:             lbcfv1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
				},
			},
		}, false
	case webhooks.StatusFail:
		interval := util.CalculateRetryInterval(op.createRsp.MinRetryDelayInSeconds)
		return &lbcfv1.TargetLoadBalancerStatus{
			Name:       op.lbName,
			Driver:     op.lbDriver,
			RetryAfter: metav1.NewTime(time.Now().Add(interval)),
			Conditions: []lbcfv1.TargetLoadBalancerCondition{
				{
					Type:               lbcfv1.LBCreated,
					Status:             lbcfv1.ConditionFalse,
					LastTransitionTime: metav1.Now(),
					Reason:             lbcfv1.ReasonCreateFailed,
					Message:            op.createRsp.Msg,
				},
			},
		}, true
	case webhooks.StatusRunning:
		interval := util.CalculateRetryInterval(op.createRsp.MinRetryDelayInSeconds)
		return &lbcfv1.TargetLoadBalancerStatus{
			Name:       op.lbName,
			Driver:     op.lbDriver,
			RetryAfter: metav1.NewTime(time.Now().Add(interval)),
			Conditions: []lbcfv1.TargetLoadBalancerCondition{
				{
					Type:               lbcfv1.LBCreated,
					Status:             lbcfv1.ConditionFalse,
					LastTransitionTime: metav1.Now(),
					Reason:             lbcfv1.ReasonCreating,
					Message:            op.createRsp.Msg,
				},
			},
		}, true
	}
	return nil, false
}

func (op lbOperation) parseEnsureResult() (*lbcfv1.TargetLoadBalancerStatus, bool) {
	if op.opType != operationEnsure {
		return nil, false
	}
	if op.err != nil {
		cond := lbcfv1.TargetLoadBalancerCondition{
			Type:               lbcfv1.LBReady,
			Status:             lbcfv1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			Reason:             lbcfv1.ReasonWebhookError,
			Message:            op.err.Error(),
		}
		newLBStatus := op.statusBeforeOperation.DeepCopy()
		newLBStatus.RetryAfter = metav1.NewTime(time.Now().Add(util.CalculateRetryInterval(20)))
		newLBStatus.Conditions = bindutil.AddOrUpdateLBCondition(newLBStatus.Conditions, cond)
		return newLBStatus, true
	}
	newLBStatus := op.statusBeforeOperation.DeepCopy()
	needRecheck := false
	switch op.ensureRsp.Status {
	case webhooks.StatusSucc:
		newLBStatus.LastSyncedAttributes = op.lbAttributes
		newLBStatus.RetryAfter = metav1.Time{}
		newLBStatus.Conditions = bindutil.AddOrUpdateLBCondition(newLBStatus.Conditions, lbcfv1.TargetLoadBalancerCondition{
			Type:               lbcfv1.LBReady,
			Status:             lbcfv1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
		})
	case webhooks.StatusFail:
		interval := util.CalculateRetryInterval(op.ensureRsp.MinRetryDelayInSeconds)
		newLBStatus.RetryAfter = metav1.NewTime(time.Now().Add(interval))
		newLBStatus.Conditions = bindutil.AddOrUpdateLBCondition(newLBStatus.Conditions, lbcfv1.TargetLoadBalancerCondition{
			Type:               lbcfv1.LBReady,
			Status:             lbcfv1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			Reason:             lbcfv1.ReasonEnsureFailed,
			Message:            op.ensureRsp.Msg,
		})
		needRecheck = true
	case webhooks.StatusRunning:
		interval := util.CalculateRetryInterval(op.ensureRsp.MinRetryDelayInSeconds)
		newLBStatus.RetryAfter = metav1.NewTime(time.Now().Add(interval))
		if !bindutil.IsLoadBalancerReady(op.statusBeforeOperation) {
			newLBStatus.Conditions = bindutil.AddOrUpdateLBCondition(newLBStatus.Conditions, lbcfv1.TargetLoadBalancerCondition{
				Type:               lbcfv1.LBReady,
				Status:             lbcfv1.ConditionFalse,
				LastTransitionTime: metav1.Now(),
				Reason:             lbcfv1.ReasonEnsuring,
				Message:            op.ensureRsp.Msg,
			})
		}
		needRecheck = true
	}
	return newLBStatus, needRecheck
}

func (op lbOperation) parseDeleteResult() (newStatus, needDelete *lbcfv1.TargetLoadBalancerStatus, recheck bool) {
	if op.opType != operationDelete {
		return nil, nil, false
	}
	if op.err != nil {
		return nil, nil, true
	}
	sts := op.statusBeforeOperation.DeepCopy()
	switch op.deleteRsp.Status {
	case webhooks.StatusSucc:
		return nil, sts, false
	case webhooks.StatusFail:
		interval := util.CalculateRetryInterval(op.ensureRsp.MinRetryDelayInSeconds)
		sts.RetryAfter = metav1.NewTime(time.Now().Add(interval))
		return sts, nil, true
	case webhooks.StatusRunning:
		return nil, nil, true
	}
	return nil, nil, false
}

func mergeStatus(oldStatus, newStatus, deletedStatus []lbcfv1.TargetLoadBalancerStatus) []lbcfv1.TargetLoadBalancerStatus {
	merged := make(map[string]lbcfv1.TargetLoadBalancerStatus)
	for _, status := range oldStatus {
		merged[status.Name] = status
	}
	for _, status := range newStatus {
		merged[status.Name] = status
	}
	for _, status := range deletedStatus {
		delete(merged, status.Name)
	}
	var ret []lbcfv1.TargetLoadBalancerStatus
	for _, status := range merged {
		ret = append(ret, status)
	}
	return ret
}

func generateBackendRecord(
	bind *lbcfv1.Bind,
	lb lbcfv1.TargetLoadBalancer,
	lbStatus lbcfv1.TargetLoadBalancerStatus,
	pod *apicorev1.Pod) []*v1beta1.BackendRecord {
	valueTrue := true
	var ret []*v1beta1.BackendRecord
	for _, port := range bind.Spec.Pods.Ports {
		ret = append(ret, &v1beta1.BackendRecord{
			ObjectMeta: metav1.ObjectMeta{
				Name: getBRName(
					bind.Name,
					lb.Name,
					pod.Name,
					pod.Status.PodIP,
					port.Protocol,
					fmt.Sprintf("%d", port.Port)),
				Namespace: bind.Namespace,
				Labels:    makeBackendLabels(lb.Driver, bind.Name, lb.Name, pod.Name),
				Finalizers: []string{
					lbcfv1.FinalizerDeregisterBackend,
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         lbcfv1.APIVersion,
						BlockOwnerDeletion: &valueTrue,
						Controller:         &valueTrue,
						Kind:               "Bind",
						Name:               bind.Name,
						UID:                bind.UID,
					},
				},
			},
			Spec: v1beta1.BackendRecordSpec{
				LBName:       lb.Name,
				LBDriver:     lb.Driver,
				LBInfo:       lbStatus.LBInfo,
				LBAttributes: lbStatus.LastSyncedAttributes,
				PodBackendInfo: &v1beta1.PodBackendRecord{
					Name: pod.Name,
					Port: v1beta1.PortSelector{
						Port:     port.Port,
						Protocol: port.Protocol,
					},
				},
				Parameters:   bind.Spec.Parameters,
				EnsurePolicy: bindutil.ConvertEnsurePolicy(bind.Spec.EnsurePolicy),
			},
		})
	}
	return ret
}

func getBRName(segments ...string) string {
	raw := strings.Join(segments, "-")
	h := md5.Sum([]byte(raw))
	return fmt.Sprintf("%x", h)
}

// makeBackendLabels generates labels for BackendRecord
func makeBackendLabels(driverName, bindName, lbName, podName string) map[string]string {
	ret := make(map[string]string)
	ret[lbcfv1.LabelDriverName] = driverName
	ret[lbcfv1.LabelBindName] = bindName
	ret[lbcfv1.LabelLBName] = lbName
	ret[lbcfv1.LabelPodName] = podName
	return ret
}

// compareBackendRecords compares expect with have and returns actions should be taken to meet the expect.
//
// The actions are return in 3 BackendRecord slices:
//
// needCreate: BackendsRecords in this slice doesn't exist in K8S and should be created
//
// needUpdate: BackendsReocrds in this slice already exist in K8S and should be update to k8s
//
// needDelete: BackendsRecords in this slice should be deleted from k8s
func compareBackendRecords(
	expect []*v1beta1.BackendRecord,
	have []*v1beta1.BackendRecord,
	doNotDelete sets.String) (needCreate, needUpdate, needDelete []*v1beta1.BackendRecord) {
	expectedRecords := make(map[string]*v1beta1.BackendRecord)
	for _, e := range expect {
		expectedRecords[e.Name] = e
	}
	haveRecords := make(map[string]*v1beta1.BackendRecord)
	for _, h := range have {
		haveRecords[h.Name] = h
	}
	for k, expect := range expectedRecords {
		have, ok := haveRecords[k]
		if !ok {
			needCreate = append(needCreate, expect)
			continue
		}
		//if !util.EqualStringMap(v.Spec.Parameters, cur.Spec.Parameters) {
		if bindutil.NeedUpdateRecord(have, expect) {
			update := have.DeepCopy()
			update.Spec = expect.Spec
			needUpdate = append(needUpdate, update)
		}
	}
	for k, v := range haveRecords {
		if _, ok := expectedRecords[k]; !ok {
			if !doNotDelete.Has(v.Name) {
				needDelete = append(needDelete, v)
			}
		}
	}
	return
}

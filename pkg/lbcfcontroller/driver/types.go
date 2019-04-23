package driver

//import (
//	"net/url"
//	"path"
//	"strings"
//	"time"
//
//	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/apis/lbcf.tke.cloud.tencent.com/v1beta1"
//)
//
//type LBDriverType string
//
//const (
//	LBDriverWebhook LBDriverType = "Webhook"
//)
//
//type WebhookName string
//
//const (
//	WebhookPrefix                  = "lbcf"
//	ValidateLBHook     WebhookName = "validateLoadBalancer"
//	CreateLBHook       WebhookName = "createLoadBalancer"
//	UpdateLBHook       WebhookName = "updateLoadBalancer"
//	DeleteLBHook       WebhookName = "deleteLoadBalancer"
//	ValidateBEHook     WebhookName = "validateBackend"
//	GenerateBEAddrHook WebhookName = "generateBackendAddr"
//	EnsureBEHook       WebhookName = "ensureBackend"
//	DeregBEHook        WebhookName = "deregisterBackend"
//	UpdateBEHook       WebhookName = "updateBackend"
//)
//
//const (
//	DefaultWebhookTimeout = 10 * time.Second
//	SystemDriverPrefix    = "lbcf-"
//)
//
//type LoadBalancerDriverInternal struct {
//	Name                 string
//	DriverType           string
//	Url                  url.URL
//	ValidateLoadBalancer *WebhookConfigInternal
//	CreateLoadBalancer   *WebhookConfigInternal
//	UpdateLoadBalancer   *WebhookConfigInternal
//	DeleteLoadBalancer   *WebhookConfigInternal
//	ValidateBackend      *WebhookConfigInternal
//	GenerateBackendAddr  *WebhookConfigInternal
//	EnsureBackend        *WebhookConfigInternal
//	DeregisterBackend    *WebhookConfigInternal
//	UpdateBackend        *WebhookConfigInternal
//}
//
//func (d *LoadBalancerDriverInternal) IsSystemDriver() bool {
//	return strings.HasPrefix(d.Name, SystemDriverPrefix)
//}
//
//type WebhookConfigInternal struct {
//	Path    string
//	Timeout time.Duration
//}
//
//func (w *WebhookConfigInternal) getUrl(u url.URL, hook string) url.URL {
//	u.Path = path.Join(WebhookPrefix, hook)
//	return u
//}
//
//func (w *WebhookConfigInternal) setTimeout(cfg *v1beta1.WebhookConfig) {
//	if cfg == nil || cfg.Timeout == nil {
//		w.Timeout = DefaultWebhookTimeout
//		return
//	}
//	// webhookConfig.Timeout is validated in validateWebhookConfig, so no error happens here
//	timeout, _ := time.ParseDuration(*cfg.Timeout)
//	w.Timeout = timeout
//}
//
//func NewLoadBalancerDriver(resource *v1beta1.LoadBalancerDriver) *LoadBalancerDriverInternal {
//	// url is validated during creating, so no error happens here
//	u, _ := url.Parse(resource.Spec.Url)
//	lbd := &LoadBalancerDriverInternal{
//		Name:                 resource.Name,
//		DriverType:           resource.Spec.DriverType,
//		Url:                  *u,
//		ValidateLoadBalancer: parseWebhooks(resource.Spec.Webhooks, ValidateLBHook),
//		CreateLoadBalancer:   parseWebhooks(resource.Spec.Webhooks, CreateLBHook),
//		UpdateLoadBalancer:   parseWebhooks(resource.Spec.Webhooks, UpdateLBHook),
//		DeleteLoadBalancer:   parseWebhooks(resource.Spec.Webhooks, DeleteLBHook),
//		ValidateBackend:      parseWebhooks(resource.Spec.Webhooks, ValidateBEHook),
//		GenerateBackendAddr:  parseWebhooks(resource.Spec.Webhooks, GenerateBEAddrHook),
//		EnsureBackend:        parseWebhooks(resource.Spec.Webhooks, EnsureBEHook),
//		DeregisterBackend:    parseWebhooks(resource.Spec.Webhooks, DeregBEHook),
//		UpdateBackend:        parseWebhooks(resource.Spec.Webhooks, UpdateBEHook),
//	}
//	return lbd
//}
//
//func parseWebhooks(webhooks *v1beta1.LoadBalancerDriverWebhook, hook WebhookName) *WebhookConfigInternal {
//	w := &WebhookConfigInternal{
//		Path:    path.Join(WebhookPrefix, string(hook)),
//		Timeout: DefaultWebhookTimeout,
//	}
//	if webhooks == nil {
//		return w
//	}
//	switch hook {
//	case ValidateLBHook:
//		w.setTimeout(webhooks.ValidateLoadBalancer)
//	case CreateLBHook:
//		w.setTimeout(webhooks.CreateLoadBalancer)
//	case UpdateLBHook:
//		w.setTimeout(webhooks.UpdateLoadBalancer)
//	case DeleteLBHook:
//		w.setTimeout(webhooks.DeleteLoadBalancer)
//	case ValidateBEHook:
//		w.setTimeout(webhooks.ValidateBackend)
//	case GenerateBEAddrHook:
//		w.setTimeout(webhooks.GenerateBackendAddr)
//	case EnsureBEHook:
//		w.setTimeout(webhooks.EnsureBackend)
//	case DeregBEHook:
//		w.setTimeout(webhooks.DeregisterBackend)
//	case UpdateBEHook:
//		w.setTimeout(webhooks.UpdateBackend)
//	}
//	return w
//}

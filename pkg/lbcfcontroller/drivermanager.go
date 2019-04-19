package lbcfcontroller

import (
	"fmt"
	"net/url"
	"path"
	"time"

	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/apis/lbcf.tke.cloud.tencent.com/v1beta1"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

type LBDriverType string

const (
	LBDriverWebhook LBDriverType = "Webhook"
)

type WebhookName string

const (
	WebhookPrefix                  = "lbcf"
	ValidateLBHook     WebhookName = "validateLoadBalancer"
	CreateLBHook       WebhookName = "createLoadBalancer"
	UpdateLBHook       WebhookName = "updateLoadBalancer"
	DeleteLBHook       WebhookName = "deleteLoadBalancer"
	ValidateBEHook     WebhookName = "validateBackend"
	GenerateBEAddrHook WebhookName = "generateBackendAddr"
	EnsureBEHook       WebhookName = "ensureBackend"
	DeregBEHook        WebhookName = "deregisterBackend"
	UpdateBEHook       WebhookName = "updateBackend"
)

const (
	DefaultWebhookTimeout = 10 * time.Second
)

type LoadBalancerDriverInternal struct {
	DriverType           string
	Url                  url.URL
	ValidateLoadBalancer *WebhookConfigInternal
	CreateLoadBalancer   *WebhookConfigInternal
	UpdateLoadBalancer   *WebhookConfigInternal
	DeleteLoadBalancer   *WebhookConfigInternal
	ValidateBackend      *WebhookConfigInternal
	GenerateBackendAddr  *WebhookConfigInternal
	EnsureBackend        *WebhookConfigInternal
	DeregisterBackend    *WebhookConfigInternal
	UpdateBackend        *WebhookConfigInternal
}

type WebhookConfigInternal struct {
	Path    string
	Timeout time.Duration
}

func (w *WebhookConfigInternal) getUrl(u url.URL, hook string) url.URL {
	u.Path = path.Join(WebhookPrefix, hook)
	return u
}

func (w *WebhookConfigInternal) setTimeout(cfg *v1beta1.WebhookConfig) {
	if cfg == nil || cfg.Timeout == nil {
		w.Timeout = DefaultWebhookTimeout
		return
	}
	// webhookConfig.Timeout is validated in validateWebhookConfig, so no error happens here
	timeout, _ := time.ParseDuration(*cfg.Timeout)
	w.Timeout = timeout
}

func NewLoadBalancerDriver(resource *v1beta1.LoadBalancerDriver) *LoadBalancerDriverInternal {
	// url is validated during creating, so no error happens here
	u, _ := url.Parse(resource.Spec.Url)
	lbd := &LoadBalancerDriverInternal{
		DriverType:           resource.Spec.DriverType,
		Url:                  *u,
		ValidateLoadBalancer: parseWebhooks(resource.Spec.Webhooks, ValidateLBHook),
		CreateLoadBalancer:   parseWebhooks(resource.Spec.Webhooks, CreateLBHook),
		UpdateLoadBalancer:   parseWebhooks(resource.Spec.Webhooks, UpdateLBHook),
		DeleteLoadBalancer:   parseWebhooks(resource.Spec.Webhooks, DeleteLBHook),
		ValidateBackend:      parseWebhooks(resource.Spec.Webhooks, ValidateBEHook),
		GenerateBackendAddr:  parseWebhooks(resource.Spec.Webhooks, GenerateBEAddrHook),
		EnsureBackend:        parseWebhooks(resource.Spec.Webhooks, EnsureBEHook),
		DeregisterBackend:    parseWebhooks(resource.Spec.Webhooks, DeregBEHook),
		UpdateBackend:        parseWebhooks(resource.Spec.Webhooks, UpdateBEHook),
	}
	return lbd
}

func parseWebhooks(webhooks *v1beta1.LoadBalancerDriverWebhook, hook WebhookName) *WebhookConfigInternal {
	w := &WebhookConfigInternal{
		Path:    path.Join(WebhookPrefix, string(hook)),
		Timeout: DefaultWebhookTimeout,
	}
	if webhooks == nil {
		return w
	}
	switch hook {
	case ValidateLBHook:
		w.setTimeout(webhooks.ValidateLoadBalancer)
	case CreateLBHook:
		w.setTimeout(webhooks.CreateLoadBalancer)
	case UpdateLBHook:
		w.setTimeout(webhooks.UpdateLoadBalancer)
	case DeleteLBHook:
		w.setTimeout(webhooks.DeleteLoadBalancer)
	case ValidateBEHook:
		w.setTimeout(webhooks.ValidateBackend)
	case GenerateBEAddrHook:
		w.setTimeout(webhooks.GenerateBackendAddr)
	case EnsureBEHook:
		w.setTimeout(webhooks.EnsureBackend)
	case DeregBEHook:
		w.setTimeout(webhooks.DeregisterBackend)
	case UpdateBEHook:
		w.setTimeout(webhooks.UpdateBackend)
	}
	return w
}

type DriverManager struct {
	store map[string]*LoadBalancerDriverInternal
}

func NewDriverManager() *DriverManager{
	return &DriverManager{}
}

func (m *DriverManager) getDriver(name string, driverType string) (*LoadBalancerDriverInternal, bool){
	d, ok := m.store[name]
	if !ok{
		return nil, false
	}
	if d.DriverType == driverType{
		return d, true
	}
	return nil, false
}

func ValidateLoadBalancerDriver(raw *v1beta1.LoadBalancerDriver) field.ErrorList {
	allErrs := field.ErrorList{}
	specPath := field.NewPath("spec")

	allErrs = append(allErrs, validateDriverType(raw.Spec.DriverType, specPath.Child("driverType"))...)
	allErrs = append(allErrs, validateDriverUrl(raw.Spec.Url, specPath.Child("url"))...)
	if raw.Spec.Webhooks != nil {
		allErrs = append(allErrs, validateWebhooks(raw.Spec.Webhooks, specPath.Child("webhooks"))...)
	}
	return allErrs
}

func validateDriverType(raw string, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if raw != string(LBDriverWebhook) {
		allErrs = append(allErrs, field.Invalid(path, raw, fmt.Sprintf("driverType must be %v", LBDriverWebhook)))

	}
	return allErrs
}

func validateDriverUrl(raw string, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if _, err := url.Parse(raw); err != nil {
		allErrs = append(allErrs, field.Invalid(path, raw, err.Error()))
	}
	return allErrs
}

func validateWebhooks(raw *v1beta1.LoadBalancerDriverWebhook, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if raw.ValidateBackend != nil {
		allErrs = append(allErrs, validateWebhookConfig(raw.ValidateLoadBalancer, path.Child("validateLoadBalancer"))...)
	}

	if raw.CreateLoadBalancer != nil {
		allErrs = append(allErrs, validateWebhookConfig(raw.CreateLoadBalancer, path.Child("createLoadBalancer"))...)
	}

	if raw.UpdateLoadBalancer != nil {
		allErrs = append(allErrs, validateWebhookConfig(raw.UpdateLoadBalancer, path.Child("updateLoadBalancer"))...)
	}

	if raw.DeleteLoadBalancer != nil {
		allErrs = append(allErrs, validateWebhookConfig(raw.DeleteLoadBalancer, path.Child("deleteLoadBalancer"))...)
	}

	if raw.ValidateBackend != nil {
		allErrs = append(allErrs, validateWebhookConfig(raw.ValidateBackend, path.Child("validateBackend"))...)
	}

	if raw.GenerateBackendAddr != nil {
		allErrs = append(allErrs, validateWebhookConfig(raw.GenerateBackendAddr, path.Child("generateBackendAddr"))...)
	}

	if raw.EnsureBackend != nil {
		allErrs = append(allErrs, validateWebhookConfig(raw.EnsureBackend, path.Child("ensureBackend"))...)
	}

	if raw.DeregisterBackend != nil {
		allErrs = append(allErrs, validateWebhookConfig(raw.DeregisterBackend, path.Child("deregisterBackend"))...)
	}
	return allErrs
}

func validateWebhookConfig(raw *v1beta1.WebhookConfig, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if raw.Timeout == nil {
		return allErrs
	}
	d, err := time.ParseDuration(*raw.Timeout)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(path, *raw.Timeout, "must be golang time.Duration"))
		return allErrs
	}
	if d.Seconds() < (5*time.Second).Seconds() || d.Seconds() > (10*time.Minute).Seconds() {
		allErrs = append(allErrs, field.Invalid(path, *raw.Timeout, "timeout must be >= 5s and <= 10m"))
		return allErrs
	}
	return allErrs
}

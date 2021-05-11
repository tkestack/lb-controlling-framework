package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	webhookCalls      *prometheus.CounterVec
	webhookErrors     *prometheus.CounterVec
	webhookFails      *prometheus.CounterVec
	webhookLatency    *prometheus.HistogramVec
	k8sOpLatency      *prometheus.HistogramVec
	keyProcessLatency *prometheus.HistogramVec
	pendingKeys       *prometheus.GaugeVec
	workingKeys       *prometheus.GaugeVec
)

const (
	labelKeyKind     = "key_kind"
	labelDriverName  = "driver_name"
	labelWebhookName = "webhook_name"
	labelK8sOpObj    = "k8s_op_obj"
	labelK8sOpType   = "k8s_op_type"
	labelCRD         = "crd"

	OpCreate       = "Create"
	OpUpdate       = "Update"
	OpUpdateStatus = "UpdateStatus"
	OpDelete       = "Delete"
)

func init() {
	webhookCalls = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "webhook_calls",
			Help: "The total number of webhook calls",
		},
		[]string{labelDriverName, labelWebhookName})

	webhookErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "webhook_errors",
			Help: "The total number of webhook errors (mostly network errors)",
		},
		[]string{labelDriverName, labelWebhookName})

	webhookFails = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "webhook_fails",
			Help: "The total number of webhooks calls that drivers responded with succ=fail or status=Fail",
		},
		[]string{labelDriverName, labelWebhookName})

	webhookLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "webhook_latency",
			Help: "webhook latencies in seconds for each non-error call",
			Buckets: []float64{0.02, 0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
				2.0, 3.0, 4.0, 5.0, 10.0, 20.0, 30.0},
		},
		[]string{labelDriverName, labelWebhookName})

	k8sOpLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "k8s_operation_latency",
			Help: "k8s operation latencies in seconds",
			Buckets: []float64{0.02, 0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
				2.0, 3.0, 4.0, 5.0, 10.0, 20.0, 30.0},
		},
		[]string{labelK8sOpObj, labelK8sOpType})

	keyProcessLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "key_process_latency",
			Help: "key process latencies in seconds",
			Buckets: []float64{0.02, 0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
				2.0, 3.0, 4.0, 5.0, 10.0, 20.0, 30.0},
		},
		[]string{labelCRD})

	pendingKeys = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pending_key",
			Help: "The number of keys waiting to be processed",
		},
		[]string{labelKeyKind})

	workingKeys = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "working_key",
			Help: "The number of keys being processed",
		},
		[]string{labelKeyKind})
}

func WebhookCallsInc(driverName, webhookName string) {
	l := prometheus.Labels{
		labelDriverName:  driverName,
		labelWebhookName: webhookName,
	}
	webhookCalls.With(l).Inc()
}

func WebhookErrorsInc(driverName, webhookName string) {
	l := prometheus.Labels{
		labelDriverName:  driverName,
		labelWebhookName: webhookName,
	}
	webhookErrors.With(l).Inc()
}

func WebhookFailsInc(driverName, webhookName string) {
	l := prometheus.Labels{
		labelDriverName:  driverName,
		labelWebhookName: webhookName,
	}
	webhookFails.With(l).Inc()
}

func WebhookLatencyObserve(driverName, webhookName string, elapsed time.Duration) {
	l := prometheus.Labels{
		labelDriverName:  driverName,
		labelWebhookName: webhookName,
	}
	webhookLatency.With(l).Observe(elapsed.Seconds())

}

func K8SOpLatencyObserve(obj, op string, elapsed time.Duration) {
	l := prometheus.Labels{
		labelK8sOpObj:  obj,
		labelK8sOpType: op,
	}
	k8sOpLatency.With(l).Observe(elapsed.Seconds())
}

func KeyProcessLatencyObserve(crd string, elapsed time.Duration) {
	l := prometheus.Labels{
		labelCRD: crd,
	}
	keyProcessLatency.With(l).Observe(elapsed.Seconds())
}

func PendingKeysSet(kind string, value float64) {
	l := prometheus.Labels{
		labelKeyKind: kind,
	}
	pendingKeys.With(l).Set(value)
}

func WorkingKeysInc(kind string) {
	l := prometheus.Labels{
		labelKeyKind: kind,
	}
	workingKeys.With(l).Inc()
}

func WorkingKeysDec(kind string) {
	l := prometheus.Labels{
		labelKeyKind: kind,
	}
	workingKeys.With(l).Dec()
}

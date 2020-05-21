The following metrics are added in v1.2.0:

|key|labels|data type|describe|
|:---:|:---:|:---:|:---:|
|pending_key|key_kind|GaugeVec|number of LBCF CRD objects waiting to be processed|
|working_key|key_kind|GaugeVec|number of LBCF CRD objects being processed|
|webhook_calls|driver_name,webhook_name|CounterVec|number of webhook calls|
|webhook_errors|driver_name,webhook_name|CounterVec|number of webhook calls that have error - network error, 404, etc.|
|webhook_fails|driver_name,webhook_name|CounterVec|number of webhook requests that drivers respond with failure|
|webhook_latency_bucket|driver_name,webhook_name|HistogramVec|time it takes to receive response from driver|

The following metrics are added in v1.3.0:

|key|labels|data type|describe|
|:---:|:---:|:---:|:---:|
|k8s_operation_latency|pending_key,k8s_op_type|HistogramVec|time it takes to finish a K8S operation(CREATE/UPDATE/DELETE)|
|key_process_latency|crd|HistogramVec|time it takes to finish processing a LBCF CRD object|

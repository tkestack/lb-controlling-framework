The following metrics are added in v1.2.0:

|key|labels|data type|describe|
|:---:|:---:|:---:|:---:|
|pending_key|key_kind|GaugeVec|number of LBCF CRD objects waiting to be processed|
|working_key|key_kind|GaugeVec|number of LBCF CRD objects being processed|
|webhook_calls|driver_name,webhook_name|CounterVec|number of webhook calls|
|webhook_errors|driver_name,webhook_name|CounterVec|number of webhook calls that have error - network error, 404, etc.|
|webhook_fails|driver_name,webhook_name|CounterVec|number of webhook requests that drivers respond with failure|
|webhook_latency_bucket|driver_name,webhook_name|HistogramVec|time it takes to receive response from driver|
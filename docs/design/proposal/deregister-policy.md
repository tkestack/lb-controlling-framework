<!-- TOC -->

- [Background](#background)
- [Solution](#solution)
    - [BackendGroup update](#backendgroup-update)
        - [deregisterPolicy](#deregisterpolicy)
        - [deregisterWebhook](#deregisterwebhook)
    - [new webhook](#new-webhook)
        - [request](#request)
        - [response](#response)

<!-- /TOC -->

# Background
1. There are factors other than Pod's `readinessProbe` that impact a Pod's `Ready` condition (see issue [#78733](https://github.com/kubernetes/kubernetes/issues/78733)), even though the connection down time is short, it's still unacceptable to our financial users. Getting rid of the `readinessProbe` is not an option because it's an important flag that indicates Pod status during Pod creating.
2. There are techniques that can minimize the down time of Pod image updating to about 2 seconds. In such scenario, it's not necessary to deregister the pod and register it back later. These techniques need special treatment so that they can be distinguished from other container exceptions.

# Solution

## BackendGroup update
There are two parameters added in `BackendGroup`: `deregisterPolicy` and `deregisterWebhook`.
```yaml
apiVersion: lbcf.tkestack.io/v1beta1
kind: BackendGroup
metadata:
  name: bg
spec:
  loadBalancers:
    - lb1
    - lb2
  deregisterPolicy: Webhook
  deregisterWebhook:
    driverName: lbcf-example-driver
    failurePolicy: DoNothing
  pods:
    ports:
      - port: 80
      - port: 90
        protocol: UDP
```

### deregisterPolicy
This the parameter users tell LBCF when their Pods should be deregistered. There are 3 available policies:  
    * `IfNotReady`: The default policy, same as K8S, pods are deregistered if `pod.status.condition[].Ready` is not `True`.  
    * `IfNotRunning`: Pods are deregistered if `pod.status.phase` is not `Running`.  
    * `Webhook`: A hightly customizable policy, driver developers may implement their own policy based on Pod. When used, `deregisterWebhook` must also be specified
**Note:** `deregisterPolicy` has no effect on registering a Pod, the standard of registering a Pod is always `pod.status.condition[].Ready` equals `True`
    
### deregisterWebhook
This parameter must be specified if `deregisterPolicy` is `Webhook`. 

`driverName`: The name of `LoadBalancerDriver`.   
`failurePolicy`: Action taken by LBCF if invoking webhook failed. There are 3 available options:  
* `DoNothing`: No Pods will be deregistered. This is the default value.
* `IfNotReady`: Same as the `IfNotReady` of `deregisterPolicy` 
* `IfNotRunning`: Same as the `IfNotRunning` of `deregisterPolicy` 

## new webhook 
A new webhook called `judgePodDeregister` is added. If the `deregisterPolicy` is `Webhook`, a request will be send to the driver.

The webhook defines as follows:

### request

```
Method: POST
Content-Type: application/json
Path: /judgePodDeregister
```

|Field|Type|Description|
|:---:|:---:|:---:|
|dryRun|bool|If lbcf-controller is running in dry-run mode|
|notReadyPods|[]*[K8S.Pod](https://kubernetes.io/docs/concepts/workloads/pods/pod/)|Pods that are neither ready nor deleting, in JSON format|

### response

|Field|Type|Required|Description|
|:---:|:---:|:---:|:---:|
|succ|bool|True|Indicates if the request is processed successfully. If not, the `failurePolicy` specified in `BackendGroup` is used|
|msg|string|False|some human readable message|
|doNotDeregister|[]*[K8S.Pod](https://kubernetes.io/docs/concepts/workloads/pods/pod/)|True|Pods that should not be deregistered|


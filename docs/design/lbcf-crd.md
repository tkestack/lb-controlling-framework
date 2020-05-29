<!-- TOC -->

- [LoadBalancerDriver](#loadbalancerdriver)
    - [LoadBalancerDriver.Status](#loadbalancerdriverstatus)
- [LoadBalancer](#loadbalancer)
    - [LoadBalancer.Status](#loadbalancerstatus)
- [BackendGroup](#backendgroup)
    - [BackendGroup.Status](#backendgroupstatus)
- [BackendRecord](#backendrecord)
    - [BackendRecord.Status](#backendrecordstatus)

<!-- /TOC -->

LBCF设计了4种CRD及其各自的Status Subresource，所有CRD皆为namespaced类型。

## LoadBalancerDriver

LoadBalancerDriver是namespaced类型CRD, 但创建在kube-system中的LoadBalancerDriver会被视为系统组件，具体表现为：

1.	创建到kube-system的LoadBalancerDriver全集群可见，且名称使用固定前缀"lbcf-"
2.	"lbcf-"为系统预留前缀，非kube-system的LoadBalancerDriver不能使用该前缀
3.	创建到其他namespace的LoadBalancerDriver仅在该namespace可见

ValidatingAdmissionWebhook的使用：

1.  触发条件：Create、Update、Delete
2.	校验基本格式（Create、Update）
3.	创建后，只允许修改webhook的timeout
4.	若要删除LoadBalancerDriver，需满足以下条件：

* driver上存在label `lbcf.tkestack.io/driver-draining:"true"`
* 所有使用该LoadBalancerDriver的LoadBalancer、BackendGroup以及BackendRecord都已删除

MutatingAdmissionWebhook的使用：未使用

**CRD结构体定义**

| Field | Type | Required| Description|
|:---:|:---:|:---:|:---|
|driverType|string|TRUE|驱动器类型，目前必须为`Webhook`|
|url| string| TRUE|Webhook server地址|
|webhooks| DriverWebhookConfig|FALSE|Webhook server的webhook配置|

**DriverWebhookConfig**

| Field | Type | Required| Description|
|:---:|:---:|:---:|:---|
|name|string|TRUE|Webhook名称，目前支持的webhook名称见[LBCF Webhook规范](lbcf-webhook-specification.md)|
|timeout| string| FALSE|webhook超时时间。最长1分钟，默认10秒|

**样例**
```yaml
apiVersion: lbcf.tkestack.io/v1beta1
kind: LoadBalancerDriver
metadata: 
  name: lbcf-clb-application 
  namespace: kube-system
spec:
  driverType: Webhook
  url: http://clb-app-driver.kube-system.svc.cluster.local
  webhooks:
    - name: validateLoadBalancer
      timeout: 15s
    - name: ensureBackend
      timeout: 1m
    # default timeout(10s) is used for other webhooks
```

### LoadBalancerDriver.Status

**CRD结构体定义**

| Field | Type | Description|
|:---:|:---:|:---|
|conditions|[]K8S.Condition|使用的Condition: `Accepted`。`Accepted`表示此LoadBalancerDriver已被lbcf-controller接受|

**样例**
```yaml
status:
  conditions:
  - lastTransitionTime: 2019-05-30T02:42:48Z
    status: "True"
    type: Accepted
```

## LoadBalancer

ValidatingAdmissionWebhook的使用：

1. 触发条件：Create、Update
2.	校验基本格式
3.	使用的LoadBalancerDriver不在draining状态（不存在label `lbcf.tkestack.io/driver-draining:"true"`)
4.	调用[validateLoadBalancer](lbcf-webhook-specification.md#validateloadbalancer)校验业务逻辑
5.	创建后，只能修改attributes和ensurePolicy  
6. 带有label `lbcf.tkestack.io/do-not-delete`时，禁止删除

MutatingAdmissionWebhook的使用：

1.	增加finalizer：

* lbcf.tkestack.io/delete-load-loadbalancer，删除前调用[deleteLoadBalancer](lbcf-webhook-specification.md#deleteloadbalancer)

**CRD结构体定义**

| Field | Type | Required| Description|
|:---:|:---:|:---:|:---|
|lbDriver|string|TRUE|使用的LoadBalancerDriver的name|
|lbSpec|map<string, string>|TRUE|负载均衡的唯一标识，用来在外部负载均衡系统中查找负载均衡实例。在临时创建负载均衡的场景中，lbSpec中的某些参数可能无法预先确定（如实例ID、监听器ID等），此时负载均衡的标识以status中的lbInfo为准，lbInfo的值由[createLoadBalancer](lbcf-webhook-specification.md#createloadbalancer)返回。**lbSpec中的字段由Webhook Server的实现者定义**|
|attributes|map<string, string>|FALSE|与唯一标识无关的负载均衡属性，例如超时时间、缴费类型等。**attributes中的字段由Webhook Server的实现者定义**|
|ensurePolicy|EnsurePolicy|FALSE|周期性检查的策略，默认不开启周期性检查|

**EnsurePolicy**

| Field | Type | Required| Description|
|:---:|:---:|:---:|:---|
|policy|string|TRUE|重试策略，支持`IfNotSucc`和`Always`，默认`IfNotSucc`。设置为`IfNotSucc`或为空时，[ensureLoadBalancer](lbcf-webhook-specification.md#ensureloadbalancer)只有在LoadBalancer.spec.attributes被修改时才会被调用；设置为`Always`时，ensureLoadBalancer会被周期性调用|
|minPeriod|string|FALSE|周期性调用的最小间隔，最少`30s`，默认`1m`。**仅当policy为`Always`时有效**|

**样例1：使用已存在的CLB实例与监听器(四层)**

```yaml
apiVersion: lbcf.tkestack.io/v1beta1
kind: LoadBalancer
metadata: 
  name: my-load-balancer-1 
  namespace: kube-system 
spec: 
  lbDriver: lbcf-clb-application
  lbSpec: 
    lbID: lb-1234
    lblID: lbl-2234
```

本例中，Webhook server的实现者定义了lbID（CLB实例ID）与lblID（监听器ID)两个参数。

在LoadBalancer被提交至K8S后，上述参数会通过[validateLoadBalancer](lbcf-webhook-specification.md#validateloadbalancer)发送给Webhook server，Webhook server会根据CLB的存在情况返回校验结果。

**样例2：临时创建CLB实例与监听器(四层)**

```yaml
apiVersion: lbcf.tkestack.io/v1beta1
kind: LoadBalancer
metadata: 
  name: my-load-balancer-1 
  namespace: kube-system 
spec: 
  lbDriver: lbcf-clb-application
  lbSpec: 
    lbVpcID: vpc-12345678
    lbListenerPort: "80"
    lbListenerProtocol: TCP
  attributes: 
    chargeType: TRAFFIC_POSTPAID_BY_HOUR
    max-bandwidth-out: 10
```

本例中，Webhook server的实现者定义了lbVpcID（CLB实例所在的VPC）、lbListenerPort（监听器端口）、lbListenerProtocol（监听器端口协议）三个参数。

在LoadBalancer被提交至K8S后，上述参数会在[createLoadBalancer](lbcf-webhook-specification.md#createloadbalancer)中发送给Webhook server，Webhook server根据参数创建CLB实例与监听器，并将实例与监听器id返回给lbcf-controller，lbcf-controller会将id记录至LoadBalancer.status中。

```yaml
status:
    conditions:
    - lastTransitionTime: 2019-05-30T10:45:26Z
      status: "True"
      type: Created
    - lastTransitionTime: 2019-05-31T07:24:43Z
      status: "True"
      type: AttributesSynced
    lbInfo:
      lbID: lb-7wf394rv
      lblID: lbl-2234
```

### LoadBalancer.Status

**CRD结构体定义**

| Field | Type | Description|
|:---:|:---:|:---|
|lbInfo|map<string, string>|负载均衡唯一标识，由[createLoadBalancer](lbcf-webhook-specification.md#createloadbalancer)返回，若其返回值为空格，则lbcf-controller会自动向其中填入LoadBalancer.spec.lbSpec的值|
|conditions|[]K8S.Condition|使用的Condition: `Created`，`AttributesSynced`。`Created`表示负载均衡已成功创建，`AttributesSynced`表示Loadbalancer.spec.attributes中的属性已同步至负载均衡|

**样例**

```yaml
status:
    conditions:
    - lastTransitionTime: 2019-05-30T10:45:26Z
      status: "True"
      type: Created
    - lastTransitionTime: 2019-05-31T07:24:43Z
      status: "True"
      type: AttributesSynced
    lbInfo:
      lbID: lb-7wf394rv
      lblID: lbl-2234
```

## BackendGroup

ValidatingAdmissionWebhook的使用：

1. 触发条件：Create、Update
2.	校验基本格式
3.	检查使用的LoadBalancer是否在正在delete，若是，则禁止创建BackendGroup
4.	调用[validateBackend](lbcf-webhook-specification.md#validatebackend)校验业务逻辑
5.	创建后，允许修改backend的选择范围、parameters与ensurePolicy，但不允许修改backend类型  
6. 带有label `lbcf.tkestack.io/do-not-delete`时，禁止删除

MutatingAdmissionWebhook的使用：未使用


**CRD结构体定义**

| Field | Type | Required| Description|
|:---:|:---:|:---:|:---|
|lbName|string|TRUE|使用的LoadBalancer的name|
|service|ServiceBackend|FALSE|被绑定至负载均衡的service配置。**service、pods、static三种配置中只能存在一种**|
|pods|PodBackend|FALSE|被绑定至负载均衡的Pod配置。**service、pods、static三种配置中只能存在一种**|
|static|[]string|FALSE|被绑定至负载均衡的静态地址配置。**service、pods、static三种配置中只能存在一种**|
|parameters|map<string, string>|TRUE|绑定backend时使用的参数|
|ensurePolicy|EnsurePolicy|FALSE|与LoadBalancer中的ensurePolicy相同|

**ServiceBackend**

| Field | Type | Required| Description|
|:---:|:---:|:---:|:---|
|name|string|TRUE|被绑定Service的name|
|port|PortSelector|TRUE|用来选择被绑定的Service Port|
|nodeSelector|map<string, string>|FALSE|用来选择被绑定的计算节点，只有label与之匹配的节点才会被绑定。为空是，选中所有节点|


**PodBackend**

| Field | Type | Required| Description|
|:---:|:---:|:---:|:---|
|port|PortSelector|TRUE|用来选择被绑定的**容器内**端口|
|byLabel|SelectPodByLabel|FALSE|通过label选择Pod|
|byName|[]string|FALSE|通过Pod.name选择Pod|

**SelectPodByLabel**

| Field | Type | Required| Description|
|:---:|:---:|:---:|:---|
|selector|map<string, string>|TRUE|被选中的Pod label|
|except|[]string|FALSE|Pod.name数组，数组中的Pod不会被选中，如果之前已被选中，则会触发该Pod的解绑流程|

**PortSelector**

| Field | Type | Required| Description|
|:---:|:---:|:---:|:---|
|portNumber|int32|TRUE|端口号|
|protocol|string|FALSE|支持`TCP`和`UDP`，默认`TCP`|

**样例1： 使用Service NodePort作为backend**

```yaml
apiVersion: tke.cloud.tencent.com/v1beta1
kind: BackendGroup
metadata: 
  name: my-lb-backend-1 
  namespace: my-namespace
spec: 
  lbName: my-load-balancer-1
  service:
    name: my-service
    port:
      portNumber: 80
      protocol: TCP 
    nodeSelector: 
      key1: value1
      key2: value2
  parameters: 
    weight: 50
```

**样例2：使用Label选择Pod，并直接将Pod绑定至负载均衡**

```yaml
apiVersion: tke.cloud.tencent.com/v1beta1
kind: BackendGroup
metadata: 
  name: my-lb-backend-2
  namespace: my-namespace
spec: 
  lbName: my-load-balancer-1
  pods:
    port:
      portNumber: 80
      protocol: TCP
    byLabel:
      app: my-web-server 
      except: 
        # Pods in except will not be registered, or will be deregistered
        - my-pod-3
        - my-pod-4
  parameters: 
    weight: 50
```

**样例3：使用name选择Pod，并直接将Pod绑定至负载均衡**

```yaml
apiVersion: tke.cloud.tencent.com/v1beta1
kind: BackendGroup
metadata: 
  name: my-lb-backend-3
  namespace: my-namespace
spec: 
  lbName: my-load-balancer-1
  pods:
    port:
      portNumber: 80
      protocol: TCP
    byName:
      # delete pod name below to deregister Pod
      - my-pod-1
      - my-pod-2
  parameters: 
    weight: 50
```

**样例4：使用静态地址作为backend**

```yaml
apiVersion: tke.cloud.tencent.com/v1beta1
kind: BackendGroup
metadata: 
  name: my-lb-backend-4
  namespace: my-namespace
spec: 
  lbName: my-load-balancer-1
  static:
  - "1.1.1.1:8080"
  - "my-web.com:8080"
  parameters: 
    weight: 50
```

### BackendGroup.Status

**CRD结构体定义**

| Field | Type | Description|
|:---:|:---:|:---|
|backends|int32|BackendGroup内backend的数量。BackendGroup中配置了service时，数量为1；配置了pods时，等于被选中的Pod数量；配置了static时，等于static数组长度|
|registerdBackends|int32|BackendGroup内已绑定backend的数量|

**样例**

```yaml
status:
  backends: 2
  registeredBackends: 2
```

## BackendRecord

BackendRecord是负载均衡中backend的抽象，每个BackendRecord对应负载均衡中的一个backend地址

BackendRecord由系统自动创建，用户不要修改其中内容。

ownerReference: 指向所属的backendGroup
    
finalizers:

* lbcf.tkestack.io/deregister-backend，解绑backend

使用的label:

| label key | Description |
|:---|:---|
|lbcf.tkestack.io/lb-driver|调用webhook所需的LoadBalancerDriver|
|lbcf.tkestack.io/lb-name|被绑定的LoadBalancer的name|
|lbcf.tkestack.io/backend-group|所属的BackendGroup|
|lbcf.tkestack.io/backend-service|BackendGroup类型为service时，此BackendRecord对应的Service的name|
|lbcf.tkestack.io/backend-pod|BackendGroup类型为pods时，此BackendRecord对应的Pod的name|
|lbcf.tkestack.io/backend-static-addr|BackendGroup类型为static时，此BackendRecord对应的静态地址|

**CRD结构体定义**

| Field | Type | Required| Description|
|:---:|:---:|:---:|:---|
|lbDriver|string|TRUE|使用的LoadBalancerDriver的name|
|lbName|string|TRUE|使用的LoadBalancer的name|
|lbInfo|map<string, string>|TRUE|当前绑定使用的负载均衡唯一标识|
|attributes|map<string, string>|FALSE|当前绑定使用的LoadBalancer.attributes|
|podBackend|PodBackendRecord|FALSE|此BackendRecord对应的Pod的信息|
|serviceBackend|ServiceBackendRecord|FALSE|此BackendRecord对应的Service的信息|
|parameters|map<string, string>|FALSE|当前绑定操作使用的参数|
|ensurePolicy|EnsurePolicy|FALSE|来自BackendGroup.spec.ensurePolicy|

**样例：PodBackend**

```yaml
apiVersion: lbcf.tkestack.io/v1beta1
kind: BackendRecord
metadata:
  creationTimestamp: 2019-05-30T10:45:26Z
  finalizers:
  - lbcf.tkestack.io/deregister-backend
  generation: 1
  labels:
    lbcf.tkestack.io/backend-group: web-backend-group
    lbcf.tkestack.io/backend-pod: web-0
    lbcf.tkestack.io/lb-driver: lbcf-god-driver
    lbcf.tkestack.io/lb-name: test-load-balancer
  name: 341953d53e36218d40b22ad9a0f67e9b
  namespace: kube-system
  ownerReferences:
  - apiVersion: lbcf.tkestack.io/v1beta1
    blockOwnerDeletion: true
    controller: true
    kind: BackendGroup
    name: web-backend-group
    uid: a2c45f2b-8284-11e9-b3e1-525400d96a00
  resourceVersion: "5260525"
  selfLink: /apis/lbcf.tkestack.io/v1beta1/namespaces/kube-system/backendrecords/341953d53e36218d40b22ad9a0f67e9b
  uid: 08e36678-82c8-11e9-b9e7-525400854cbb
spec:
  lbAttributes: null
  lbDriver: lbcf-god-driver
  lbInfo:
    lbID: lb-7wf394rv
  lbName: test-load-balancer
  parameters:
    weight: "100"
  podBackend:
    name: web-0
    port:
      portNumber: 80
      protocol: TCP
```

### BackendRecord.Status

**CRD结构体定义**

| Field | Type | Description|
|:---:|:---:|:---|
|backendAddr|string|被绑定backend的地址，来自[generateBackendAddr](lbcf-webhook-specification.md#generatebackendaddr)|
|injectedInfo|map<string, string>|绑定成功时由[ensureBackend](lbcf-webhook-specification.md#ensureBackend)返回的内容|
|conditions|[]K8S.Condition|使用的Condition：`Registered`。`Registered`表示backend已绑定成功|

**样例**

```yaml
status:
  backendAddr: '{"listenerID":"lbl-d7bbw7on","lbPort":{"port":10000,"protocol":2},"eniIP":"10.0.3.244","backendPort":80}'
  conditions:
  - lastTransitionTime: 2019-05-30T10:45:28Z
    message: ""
    status: "True"
    type: Registered
  injectedInfo: null
```
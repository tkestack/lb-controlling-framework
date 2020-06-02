<!-- TOC -->

- [LoadBalancerDriver](#loadbalancerdriver)
    - [LoadBalancerDriver.Status](#loadbalancerdriverstatus)
- [LoadBalancer](#loadbalancer)
    - [范例](#范例)
        - [范例1：使用已存在的负载均衡](#范例1使用已存在的负载均衡)
        - [范例2：动态创建负载均衡](#范例2动态创建负载均衡)
        - [范例3：由系统管理员限定每个namespace可用的负载均衡](#范例3由系统管理员限定每个namespace可用的负载均衡)
    - [LoadBalancer.Status](#loadbalancerstatus)
- [BackendGroup](#backendgroup)
    - [范例](#范例-1)
        - [范例1：使用Service NodePort作为backend](#范例1使用service-nodeport作为backend)
        - [范例2：使用Label选择Pod，并直接将Pod绑定至负载均衡](#范例2使用label选择pod并直接将pod绑定至负载均衡)
        - [范例3：使用name选择Pod，并直接将Pod绑定至负载均衡](#范例3使用name选择pod并直接将pod绑定至负载均衡)
        - [范例4：使用静态地址作为backend](#范例4使用静态地址作为backend)
        - [范例5：同一个后端绑定至多个负载均衡](#范例5同一个后端绑定至多个负载均衡)
        - [范例6：自定义解绑条件](#范例6自定义解绑条件)
    - [BackendGroup.Status](#backendgroupstatus)
- [BackendRecord](#backendrecord)
    - [BackendRecord.Status](#backendrecordstatus)

<!-- /TOC -->

LBCF设计了4种CRD及其各自的Status Subresource，所有CRD皆为namespaced类型。

# LoadBalancerDriver

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

## LoadBalancerDriver.Status

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

# LoadBalancer

ValidatingAdmissionWebhook的使用：

1. 触发条件：Create、Update
2.	校验基本格式
3.	使用的LoadBalancerDriver不在draining状态（不存在label `lbcf.tkestack.io/driver-draining:"true"`)
4.	调用[validateLoadBalancer](lbcf-webhook-specification.md#validateloadbalancer)校验业务逻辑
5.	创建后，只能修改attributes和ensurePolicy  
6. 带有label `lbcf.tkestack.io/do-not-delete`时，禁止删除
7. `name`以`lbcf-`开头的`LoadBalancer`可在多个namespace中共享，见[范例3: 由系统管理员限定每个namespace可用的负载均衡](#范例3-由系统管理员限定每个namespace可用的负载均衡)

MutatingAdmissionWebhook的使用：

1.	增加finalizer：

* lbcf.tkestack.io/delete-load-loadbalancer，删除前调用[deleteLoadBalancer](lbcf-webhook-specification.md#deleteloadbalancer)

**CRD结构体定义**

| Field | Type | Required| Description|
|:---:|:---:|:---:|:---|
|lbDriver|string|TRUE|使用的LoadBalancerDriver的name|
|lbSpec|map<string, string>|TRUE|负载均衡的唯一标识，用来在外部负载均衡系统中查找负载均衡实例。在临时创建负载均衡的场景中，lbSpec中的某些参数可能无法预先确定（如实例ID、监听器ID等），此时负载均衡的标识以status中的lbInfo为准，lbInfo的值由[createLoadBalancer](lbcf-webhook-specification.md#createloadbalancer)返回。**lbSpec中的字段由Webhook Server的实现者定义**|
|attributes|map<string, string>|FALSE|与唯一标识无关的负载均衡属性，例如超时时间、缴费类型等。**attributes中的字段由Webhook Server的实现者定义**|
|scope|[]string|FALSE|设置LoadBalancer跨namespace共享的范围|
|ensurePolicy|EnsurePolicy|FALSE|周期性检查的策略，默认不开启周期性检查|

**EnsurePolicy**

| Field | Type | Required| Description|
|:---:|:---:|:---:|:---|
|policy|string|TRUE|重试策略，支持`IfNotSucc`和`Always`，默认`IfNotSucc`。设置为`IfNotSucc`或为空时，[ensureLoadBalancer](lbcf-webhook-specification.md#ensureloadbalancer)只有在LoadBalancer.spec.attributes被修改时才会被调用；设置为`Always`时，ensureLoadBalancer会被周期性调用|
|minPeriod|string|FALSE|周期性调用的最小间隔，最少`30s`，默认`1m`。**仅当policy为`Always`时有效**|

## 范例
### 范例1：使用已存在的负载均衡

假设被对接的外部系统可以使用(`lbID`,`lblID`)二元组唯一确定一个负载均衡，则可以Webhook server可以在`lbSpec`中定义`lbID`和`lblID`两个参数，并要求用户向K8S中提交如下内容：

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

上述YAML被提交至K8S后，其中的参数会通过[validateLoadBalancer](lbcf-webhook-specification.md#validateloadbalancer)发送给Webhook server，Webhook server可以在接口实现中校验负载均衡是否存在。

### 范例2：动态创建负载均衡
大部分云服务商都允许动态申请负载均衡，假设云服务商的API规定申请负载均衡时需提交`lbVpcID`, `lbListenerPort`和`lbListenerProtocol`三个必填参数以及一个可选参数`chargeType`，并且申请成功后会返回负载均衡的唯一ID（`lbID`,`lblID`），则Webhook server可以将三个必填参数定义在`lbSpec`，将可选参数定义在`attributes`，最后在申请成功时将（`lbID`,`lblID`）返回给LBCF做持久化保存。  
使用时，用户需要向K8S中提交以下内容：
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
```
上述YAML被提交至K8S后，LBCF会通过[validateLoadBalancer](lbcf-webhook-specification.md#validateloadbalancer)将其中参数发送给Webhook server校验，并通过[createLoadBalancer](lbcf-webhook-specification.md#createloadbalancer)通知Webhook server去申请负载均衡，申请完成后，Webhook Server可将（`lbID`,`lblID`）放入响应消息，LBCF会将其存储在`status`中，如下：
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

### 范例3：由系统管理员限定每个namespace可用的负载均衡
很多集群都是通过namespace进行资源共享和隔离的，有时集群管理员希望能为每个namespace预先创建一些负载均衡，这些负载均衡可在namespace中使用，但不允许普通用户删除或修改配置。在LBCF中，这一场景是通过`LoadBalancer`中的`scope`参数实现的。`scope`的使用规则为：
* `scope`中填写此`LoadBalancer`允许被使用的namespace，为`*`时代表所有namespace（包括尚未创建的）
* `scope`为空时，`LoadBalancer`只能在所在namespace中被使用
* 若`scope`不为空，则`LoadBalancer`的`name`必须以`lbcf-`开头，并且必须被创建在`kube-system`中（这通常意味着创建者是集群管理员）
* 非`kube-system`的namespace中不能创建`name`以`lbcf-`开头的`LoadBalancer`

通过以下YAML，集群管理员可以将`LoadBalancer`的使用范围限制在`ns-a`和`ns-b`两个namespace:
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
  scope:
  - ns-a
  - ns-b
```

## LoadBalancer.Status

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

# BackendGroup

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
|loadBalancers|[]string|TRUE|使用的LoadBalancer的name|
|service|ServiceBackend|FALSE|被绑定至负载均衡的service配置。**service、pods、static三种配置中只能存在一种**|
|pods|PodBackend|FALSE|被绑定至负载均衡的Pod配置。**service、pods、static三种配置中只能存在一种**|
|static|[]string|FALSE|被绑定至负载均衡的静态地址配置。**service、pods、static三种配置中只能存在一种**|
|parameters|map<string, string>|TRUE|绑定backend时使用的参数|
|deregisterPolicy|string|FALSE|Pod解绑条件，仅当`pods`不为nil时有效，可选的值为`IfNotReady`、`IfNotRunning`、`Webhook`。不填时默认为`IfNotReady`。详见文档[自定义解绑条件设计](/docs/design/proposal/deregister-policy.md)|
|deregisterWebhook|DeregisterWebhookSpec|FALSE|通过Webhook判断Pod是否解绑，仅当`deregisterPolicy`为`Webhook`时有效。详见文档[自定义解绑条件设计](/docs/design/proposal/deregister-policy.md)|
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
|ports|[]PortSelector|TRUE|用来选择被绑定的**容器内**端口|
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
|port|int32|TRUE|端口号|
|protocol|string|FALSE|支持`TCP`和`UDP`，默认`TCP`|

**DeregisterWebhookSpec**
| Field | Type | Required| Description|
|:---:|:---:|:---:|:---|
|driverName|string|TRUE|接受webhook请求的server，给定名称的`LoadBalancerDriver`必须已经存在|
|failurePolicy|string|FALSE|webhook请求失败时的处理策略，可选值为`DoNothing`、`IfNotReady`、`IfNotRunning`，默认为`DoNothing`。详见文档[自定义解绑条件设计](/docs/design/proposal/deregister-policy.md)|

## 范例
### 范例1：使用Service NodePort作为backend
假定有以下Service
```
NAME    TYPE        CLUSTER-IP      EXTERNAL-IP     PORT(S)         AGE     SELECTOR
foo-svc NodePort    192.168.30.7    <none>          80:32760/TCP    21d     app=foo
```

可以使用如下`BackendGroup`将`Service`的`80`端口对应的nodePort绑定至负载均衡:
```yaml
apiVersion: tke.cloud.tencent.com/v1beta1
kind: BackendGroup
metadata: 
  name: my-lb-backend-1 
  namespace: my-namespace
spec: 
  loadBalancers: 
    - my-load-balancer-1
  service:
    name: my-service
    port:
      port: 80
      protocol: TCP 
    nodeSelector: 
      my-node-label: foo
  parameters: 
    weight: 50
```
PS: `nodeSelector`中可以按label选择node，只有被选择的node才会被绑定

### 范例2：使用Label选择Pod，并直接将Pod绑定至负载均衡

```yaml
apiVersion: tke.cloud.tencent.com/v1beta1
kind: BackendGroup
metadata: 
  name: my-lb-backend-2
  namespace: my-namespace
spec: 
  loadBalancers: 
    - my-load-balancer-1
  pods:
    ports:
      - port: 80
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

### 范例3：使用name选择Pod，并直接将Pod绑定至负载均衡

```yaml
apiVersion: tke.cloud.tencent.com/v1beta1
kind: BackendGroup
metadata: 
  name: my-lb-backend-3
  namespace: my-namespace
spec: 
  loadBalancers: 
    - my-load-balancer-1
  pods:
    port:
      port: 80
      protocol: TCP
    byName:
      # delete pod name below to deregister Pod
      - my-pod-1
      - my-pod-2
  parameters: 
    weight: 50
```

### 范例4：使用静态地址作为backend
如果你了解自己使用的Webhook server，也可以使用静态的后端地址，LBCF会将`static`中的地址发送给Webhook server，并直接绑定到负载均衡上。

```yaml
apiVersion: tke.cloud.tencent.com/v1beta1
kind: BackendGroup
metadata: 
  name: my-lb-backend-4
  namespace: my-namespace
spec: 
  loadBalancers: 
    - my-load-balancer-1
  static:
  - "1.1.1.1:8080"
  - "my-web.com:8080"
  parameters: 
    weight: 50
```

### 范例5：同一个后端绑定至多个负载均衡
在某些场景下，一个后端需要绑定至多个负载均衡，这些负载均衡可能是同一类型，也可能是不同类型。在LBCF中，只需将多个`LoadBalancer`的`name`填入`BackendGroup`即可，`BackendGroup`选中的每个后端都将绑定至所有负载均衡。

```yaml
apiVersion: tke.cloud.tencent.com/v1beta1
kind: BackendGroup
metadata: 
  name: my-bg
spec: 
  loadBalancers:
    - lb-1 
    - lbcf-shared-lb 
  pods:
    ports:
      - port: 80
        protocol: TCP
      - port: 90
        protocol: UDP
    byName:
      - pod-0
      - pod-1
```
在上述YAML中，我们指定了`lb-1`和`lbcf-shared-lb`两个负载均衡，同时在`ports`中指定了2个容器端口，则最终的绑定关系如下：

|负载均衡|后端服务器|
|:---:|:---:|
|lb-1|pod-0:80, pod-0:90, pod-1:80, pod-1:90|
|lbcf-shared-lb|pod-0:80, pod-0:90, pod-1:80, pod-1:90|

### 范例6：自定义解绑条件
在K8S中，Pod的`Ready` condition被用来判断Pod是否应当被解绑，LBCF在完整支持这种判断条件的同时，还提供了`IfNotRunning`和`Webhook`两种判断方式，用户可以根据自己的业务特点决定什么时候Pod应当被解绑。  

假定用户已经注册了如下Webhook server：
```
NAMESPACE     NAME                       AGE
kube-system   lbcf-foo-driver            21d
```
则用户可提交以下YAML以通过`lbcf-foo-driver`判断Pod是否应当解绑。
```yaml
apiVersion: lbcf.tkestack.io/v1beta1
kind: BackendGroup
metadata:
  name: bg
spec:
  loadBalancers:
    - lb1
  deregisterPolicy: Webhook
  deregisterWebhook:
    driverName: foo-driver
    failurePolicy: DoNothing
  pods:
    ports:
      - port: 90
        protocol: UDP
```
自定义解绑条件详见文档[自定义解绑条件设计](/docs/design/proposal/deregister-policy.md)。

## BackendGroup.Status

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

# BackendRecord

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
      port: 80
      protocol: TCP
```

## BackendRecord.Status

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
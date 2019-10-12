<!-- TOC -->

- [部署Webhook server](#部署webhook-server)
- [定义LoadBalancer](#定义loadbalancer)
- [查看LoadBalancer状态](#查看loadbalancer状态)
- [定义BackendGroup](#定义backendgroup)
- [查看BackendRecord](#查看backendrecord)
- [强制删除BackendRecord](#强制删除backendrecord)

<!-- /TOC -->

## 部署Webhook server

在LBCF中，Webhook server负责调用负载均衡api。由于每种负载均衡都有自己的Webhook server，所以使用LBCF的第一步就是把Webhook server的部署信息告知LBCF，这一操作是通过向K8S中提交[LoadBalancerDriver](https://tkestack.io/lb-controlling-framework/blob/master/docs/design/lbcf-crd.md#loadbalancerdriver)对象完成的。

从[LoadBalancerDriver](https://tkestack.io/lb-controlling-framework/blob/master/docs/design/lbcf-crd.md#loadbalancerdriver)的定义可知，其中最重要的信息为Webhook server地址，即`spec.url`部分。
LBCF对webhook server的部署位置没有限制，既可以作为容器部署在集群内，也可以部署在集群外部的节点上。

clb-driver是本人开发的用于对接共有云CLB的webhook server，该项目使用了容器化部署，即使用Deployment部署Webhook server，为Deployment创建Service并将Service地址告知LBCF。

下述YAML为部署clb-driver使用的Service，Service的80端口即webhook server用来提供服务的端口

```yaml
apiVersion: v1
kind: Service
metadata:
  labels:
  name: lbcf-clb-driver
  namespace: kube-system
spec:
  ports:
    - name: insecure
      port: 80
      targetPort: 80
  selector:
    lbcf.tke.cloud.tencent.com/component: lbcf-clb-driver
  sessionAffinity: None
  type: ClusterIP
```

从下面的LoadBalancerDriver中可以看到，Webhook server的地址为Service地址，该地址可以被K8S集群内部DNS（kube-dns或core-dns）解析

```yaml
apiVersion: lbcf.tke.cloud.tencent.com/v1beta1
kind: LoadBalancerDriver
metadata:
  name: lbcf-clb-driver
  namespace: kube-system
spec:
  driverType: Webhook
  url: "http://lbcf-clb-driver.kube-system.svc"
```

## 定义LoadBalancer

[LoadBalancer](https://tkestack.io/lb-controlling-framework/blob/master/docs/design/lbcf-crd.md#loadbalancer)描述了被操作的负载均衡的信息，其中主要包含以下内容：

1. 应当由哪个Webhook server执行操作(spec.lbDriver)
2. 负载均衡在外部系统中的唯一标识是什么(spec.lbSpec)
3. 负载均衡有哪些与标识无关的属性(spec.attributes)

一旦LoadBalancer对象创建成功，1和2中的内容就会被禁止修改，只有3中的内容可以被随时修改。

LBCF作为一个开放框架，没有对2和3中的内容进行限制，其中的信息完全由Webhook server定义和解析，下述YAML展示了clb-driver定义的部分参数:

```yaml
apiVersion: lbcf.tke.cloud.tencent.com/v1beta1
kind: LoadBalancer
metadata:
  name: test-clb-load-balancer
  namespace: kube-system
spec:
  lbDriver: lbcf-clb-driver
  lbSpec:
    vpcID: vpc-b5hcoxj4
    loadBalancerType: "OPEN"
    listenerPort: "9999"
    listenerProtocol: "HTTP"
    domain: "mytest.com"
    url: "/index.html"
  ensurePolicy:
    policy: Always
```

向K8S提交LoadBalancer对象后，LBCF在本地对LoadBalancer进行基本校验后，会先后调用Webhook server的[validateLoadBalancer](https://tkestack.io/lb-controlling-framework/blob/master/docs/design/lbcf-webhook-specification.md#validateloadbalancer)与[createLoadBalancer](https://tkestack.io/lb-controlling-framework/blob/master/docs/design/lbcf-webhook-specification.md#createloadbalancer)方法。

在validateLoadBalancer中，Webhook server可以校验并拒绝本次LoadBalancer对象的创建。

例如，clb-driver支持使用已有的CLB实例，当LoadBalancer中指定的CLB不存在时，clb-driver会拒绝该次创建。

```yaml
apiVersion: lbcf.tke.cloud.tencent.com/v1beta1
kind: LoadBalancer
metadata:
  name: a-lb-that-not-exist
  namespace: kube-system
spec:
  lbDriver: lbcf-clb-driver
  lbSpec:
    loadBalancerID: "lb-notexist"
    listenerPort: "9999"
    listenerProtocol: "HTTP"
    domain: "mytest.com"
    url: "/index.html"
```
使用上述YAML时，clb-driver会调用[云API](https://cloud.tencent.com/document/api/214/30685)检查loadBalancerID中的`lb-notexist`是否存在，如果不存在，会拒绝创建：

```bash
kubectl apply -f lb-not-exist.yaml
Error from server: error when creating "lb-not-exist.yaml": admission webhook "lb.lbcf.tke.cloud.tencent.com" denied the request: invalid LoadBalancer: clb instance lb-notexist not found
```

## 查看LoadBalancer状态

若validateLoadBalancer校验返回成功，则LoadBalancer对象会被写入K8S集群，此时，LBCF会调用[createLoadBalancer](https://tkestack.io/lb-controlling-framework/blob/master/docs/design/lbcf-webhook-specification.md#createloadbalancer)进行负载均衡的创建（webhook server可直接返回成功以跳过此流程）

依旧以clb-driver为例，这次我们使用下述YAML临时创建一个新的七层CLB:
```yaml
apiVersion: lbcf.tke.cloud.tencent.com/v1beta1
kind: LoadBalancer
metadata:
  name: test-clb-load-balancer
  namespace: kube-system
spec:
  lbDriver: lbcf-clb-driver
  lbSpec:
    vpcID: vpc-b5hcoxj4
    loadBalancerType: "OPEN"
    listenerPort: "9999"
    listenerProtocol: "HTTP"
    domain: "mytest.com"
    url: "/index.html"
```

使用`kubectl describe`命令查看一下LoadBalancer的状态，可得以下结果：
```bash
[root@10-0-3-16 clb-driver]# kubectl describe loadbalancer -n kube-system test-clb-load-balancer
Name:         test-clb-load-balancer
Namespace:    kube-system
Labels:       <none>
Annotations:  kubectl.kubernetes.io/last-applied-configuration:
                {"apiVersion":"lbcf.tke.cloud.tencent.com/v1beta1","kind":"LoadBalancer","metadata":{"annotations":{},"name":"test-clb-load-balancer","nam...
API Version:  lbcf.tke.cloud.tencent.com/v1beta1
Kind:         LoadBalancer
Metadata:
  Creation Timestamp:  2019-06-13T12:48:44Z
  Finalizers:
    lbcf.tke.cloud.tencent.com/delete-load-loadbalancer
  Generation:        1
  Resource Version:  8574359
  Self Link:         /apis/lbcf.tke.cloud.tencent.com/v1beta1/namespaces/kube-system/loadbalancers/test-clb-load-balancer
  UID:               94518f90-8dd9-11e9-b3e1-525400d96a00
Spec:
  Ensure Policy:
    Policy:   Always
  Lb Driver:  lbcf-clb-driver
  Lb Spec:
    Domain:              mytest.com
    Listener Port:       9999
    Listener Protocol:   HTTP
    Load Balancer Type:  OPEN
    URL:                 /index.html
    Vpc ID:              vpc-b5hcoxj4
Status:
  Conditions:
    Last Transition Time:  2019-06-13T12:49:19Z
    Status:                True
    Type:                  Created
    Last Transition Time:  2019-06-13T12:49:19Z
    Status:                True
    Type:                  AttributesSynced
  Lb Info:
    Domain:             mytest.com
    Listener Port:      9999
    Listener Protocol:  HTTP
    Load Balancer ID:   lb-6xm34m0z
    URL:                /index.html
Events:
  Type    Reason                     Age   From             Message
  ----    ------                     ----  ----             -------
  Normal  RunningCreateLoadBalancer  33s   lbcf-controller  msg: creating CLB instance
  Normal  RunningCreateLoadBalancer  23s   lbcf-controller  msg: creating listener
  Normal  RunningCreateLoadBalancer  12s   lbcf-controller  msg: creating forward rule
  Normal  SuccCreateLoadBalancer     2s    lbcf-controller  Successfully created load balancer
```
在返回结果的Events部分，我们可以看到clb-driver依次创建了CLB实例、监听器与7层转发规则并最后返回了成功。

*注：此处之所以有4个Event，是因为clb-driver实现createLoadBalancer时使用了异步操作，LBCF一共调用了4次createLoadBalancer*

另一方面，从Status中的Lb Info中可以看到原本lbSpec中的`vpcID: vpc-b5hcoxj4`与`loadBalancerType: "OPEN"`被替换成了Load Balancer ID `lb-6xm34m0z`，替换的原因在于Load Balancer ID在云API是负载均衡实例的唯一标识，但完成创建之前，我们无法预先获取该ID，因此lbSpec中填写的是创建实例所需的参数，而lbInfo中的ID是由clb-driver在创建完成后才写入的。

当LoadBalancer被删除时，绑定在其下的所有backend都会解绑。

[BackendRecord](https://tkestack.io/lb-controlling-framework/blob/master/docs/design/lbcf-crd.md#backendrecord)会被解绑

## 定义BackendGroup

[BackendGroup](https://tkestack.io/lb-controlling-framework/blob/master/docs/design/lbcf-crd.md#backendgroup)描述了被绑定backend的信息，主要包含如下内容：

1. backend需要被绑定在哪个LoadBalancer(spec.lbName)
2. 哪些backend需要被绑定(spec.service, spec.pods, spec.static)
3. 绑定backend时需要使用哪些参数(spec.parameters)

与LoadBalancer类似，3中的内容也是完全由webhook server自定义的，下述YAML展示了clb-driver对权重的支持：

```yaml
apiVersion: lbcf.tke.cloud.tencent.com/v1beta1
kind: BackendGroup
metadata:
  name: web-svc-backend-group
  namespace: kube-system
spec:
  lbName: test-clb-load-balancer
  service:
    name: svc-test
    port:
      portNumber: 80
  parameters:
    weight: "36"
```
当用户修改spec.parameters.weight的值时，CLB中对应backend的权重会产生相应改变。

BackendGroup目前支持了3种backend类型，除上面YAML使用的service类型外，还有pods与static类型。如果是pods类型，LBCF会将Pod直接绑定至CLB（数据面由网络自行保证），下面的YAML为clb-driver项目支持的pod类型BackendGroup：

```yaml
apiVersion: lbcf.tke.cloud.tencent.com/v1beta1
kind: BackendGroup
metadata:
  name: web-pod-backend-group
  namespace: kube-system
  labels:
    lbcf.tke.cloud.tencent.com/lb-name: test-clb-load-balancer
spec:
  lbName: test-clb-load-balancer
  pods:
    port:
      portNumber: 80
    byLabel:
      selector:
        app: nginx
  parameters:
    weight: "18"
```

## 查看BackendRecord

[BackendRecord](https://tkestack.io/lb-controlling-framework/blob/master/docs/design/lbcf-crd.md#backendrecord)由LBCF自动创建并管理，其中记录了被绑定的backend的信息（1个backend对应1个BackendRecord），用户应避免手动操作此类对象。

BackendRecord的Status中记录了backend的当前状态，包括backend地址、是否已完成绑定以及每次调用webhook的结果。

```bash
[root@10-0-3-16 clb-driver]# kubectl describe backendrecord -n kube-system
Name:         dea99df137c5b3d94d5e858a7c3ca778
Namespace:    kube-system
Labels:       lbcf.tke.cloud.tencent.com/backend-group=web-svc-backend-group
              lbcf.tke.cloud.tencent.com/backend-service=svc-test
              lbcf.tke.cloud.tencent.com/lb-driver=lbcf-clb-driver
              lbcf.tke.cloud.tencent.com/lb-name=test-clb-load-balancer
Annotations:  <none>
API Version:  lbcf.tke.cloud.tencent.com/v1beta1
Kind:         BackendRecord
Metadata:
  Creation Timestamp:  2019-06-13T13:23:05Z
  Finalizers:
    lbcf.tke.cloud.tencent.com/deregister-backend
  Generation:  1
  Owner References:
    API Version:           lbcf.tke.cloud.tencent.com/v1beta1
    Block Owner Deletion:  true
    Controller:            true
    Kind:                  BackendGroup
    Name:                  web-svc-backend-group
    UID:                   46f7f7b5-8daf-11e9-b3e1-525400d96a00
  Resource Version:        8580045
  Self Link:               /apis/lbcf.tke.cloud.tencent.com/v1beta1/namespaces/kube-system/backendrecords/dea99df137c5b3d94d5e858a7c3ca778
  UID:                     60aee0ff-8dde-11e9-b409-525400b94ff4
Spec:
  Lb Attributes:  <nil>
  Lb Driver:      lbcf-clb-driver
  Lb Info:
    Domain:             mytest.com
    Listener Port:      9999
    Listener Protocol:  HTTP
    Load Balancer ID:   lb-7wf394rv
    URL:                /index.html
  Lb Name:              test-clb-load-balancer
  Parameters:
    Weight:  36
  Service Backend:
    Name:       svc-test
    Node Name:  10.0.3.3
    Node Port:  30200
    Port:
      Port Number:  80
      Protocol:     TCP
Status:
  Backend Addr:  {"instanceID":"ins-ddyckir3","eIP":"","port":30200}
  Conditions:
    Last Transition Time:  2019-06-13T13:23:24Z
    Message:
    Status:                True
    Type:                  Registered
  Injected Info:           <nil>
Events:
  Type    Reason                Age   From             Message
  ----    ------                ----  ----             -------
  Normal  SuccGenerateAddr      64s   lbcf-controller  addr: {"instanceID":"ins-ddyckir3","eIP":"","port":30200}
  Normal  RunningEnsureBackend  59s   lbcf-controller  msg: requestID: f5291b17-122d-406d-917a-6f48bfc8b9b4
  Normal  SuccEnsureBackend     46s   lbcf-controller  Successfully ensured backend
```

Status中的Backend Addr是被绑定backend的地址，该地址由[generateBackend](https://tkestack.io/lb-controlling-framework/blob/master/docs/design/lbcf-webhook-specification.md#generatebackendaddr)返回。本例中，我们绑定的是Service Node，但由于[云API](https://cloud.tencent.com/document/api/214/30676)中只能填写instanceID，因此clb-driver通过查询API把节点IP转换为instanceID，并使用instanceID作为backend地址。

与LoadBalancer类似，clb-driver实现[ensureBackend](https://tkestack.io/lb-controlling-framework/blob/master/docs/design/lbcf-webhook-specification.md#ensurebackend)也使用了异步操作，所以Events中有2次ensureBackend的调用结果

当LoadBalancer或BackendGroup被删除时，BackendRecord会被自动解绑并删除

## 强制删除BackendRecord

通常情况下，删除BackendRecord会触发backend的解绑，但在某些情况下，运维人员可能需要在不解绑backend的前提下删除BackendRecord。

若需强制删除BackendRecord，需按下述步骤进行操作：

1. 删除所有BackendRecord中的Finalizer `lbcf.tke.cloud.tencent.com/deregister-backend`
2. 删除BackendGroup或LoadBalancer

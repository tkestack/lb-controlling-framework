# Load Balancer Controlling Framework (LBCF)

LBCF是一款部署在Kubernetes内的通用负载均衡控制面框架，实现了可靠的一致性保证与灵活的扩展接口。通过实现LBCF提供的扩展接口，您可以快速实现可靠的负载均衡/名字服务对接。

LBCF自上线以来，没有发生过容器的错误绑定或错误解绑，这一切都源自LBCF设计了可靠的一致性保证机制，详见文档: [LBCF的一致性保证](/docs/design/lbcf-consistency-design.md)

可扩展性是LBCF的核心，为此，LBCF针对负载均衡操作特点，在Pod生命周期中设计了9个可实现自定义逻辑的Webhook，webhook定义见文档：[LBCF CRD设计](/docs/design/lbcf-crd.md)

---

## 安装LBCF

见文档:[安装LBCF](/docs/install.md)

---
##  基于LBCF实现业务逻辑
得益于LBCF的可靠性与扩展能力，我们的用户实现了众多高度定制化的业务需求。在使用LBCF前，这些需求往往需要单独开发controller，并经过漫长的周期才能逐渐稳定。

### 将Pod注册至第三方系统（负载均衡/名字服务/其他）
标题中提到的"第三方系统"并不特指任何系统，只要其提供了API，并且业务需求符合"Pod启动时注册，Pod结束时注销"的行为模式，就都可以使用LBCF。LBCF的一些用户就实现了将Pod注册至自己内部系统的功能。

#### Pod生命周期监听
使用LBCF后，您无需自行实现Pod生命周期的监听，您只需在接收端（在LBCF中称为Driver）实现自己的业务逻辑即可。LBCF会在以下时刻触发Webhook：
* Pod非Ready --> Pod Ready  
* Pod Ready --> Pod非Ready  
* Pod 被设置`deletionTimestamp`  
* Pod Ready，但不再被用户指定为需要注册
* Pod Ready，并重新被用户指定为需要注册    
PS: 当Pod发生扩/缩容时，LBCF也会自动响应。

#### 调用第三方API
当Webhook触发时，Webhook请求中会携带相关Pod的信息，您可以按需进行信息处理与第三方API调用：  
[generateBackendAddr](/docs/design/lbcf-webhook-specification.md#generatebackendaddr): 用于生成注册Pod时使用的参数  
[ensureBackend](/docs/design/lbcf-webhook-specification.md#ensurebackend): 用于将`generateBackendAddr`生成的参数注册至第三方系统  
[deregisterBackend](/docs/design/lbcf-webhook-specification.md#deregisterbackend): 用于将`generateBackendAddr`生成的参数从第三方系统注销  

#### 第三方系统的自定义参数
调用第三方系统API时必然要填写一些系统相关参数，这些参数不可能完全来自K8S。因此，LBCF允许Driver实现自定义参数，并支持Driver对用户输入进行校验。

#### 面向平台使用者的系统运维
阅读系统日志是困难且痛苦的，为此，LBCF将Webhook的响应与系统关键状态都以K8S event的形式输出至了相关CRD对象中，用户使用`kubectl describe`命令就可以看到系统状态。
更重要的是，这些event中包含的是Driver返回的业务信息，而不是LBCF的内部信息，即便用户对LBCF一无所知，依然可以明白系统在何时发生了什么。  
以下event摘自真实环境，其中`addr`中的内容就是由Driver定义的
```yaml
Events:
  Type    Reason             Age   From             Message
  ----    ------             ----  ----             -------
  Normal  SuccGenerateAddr   22s   lbcf-controller  addr: {"instanceID":"","eIP":"10.0.3.244","port":80}
  Normal  SuccEnsureBackend  16s   lbcf-controller  Successfully ensured backend
```

### 可选的Pod解绑条件
K8S中对Pod是否需要解绑的判断依据主要是Pod的`Ready`条件和`deletionTimestamp`，LBCF完全支持这种判断，同时还提供了2种新的判断条件：
* 按Running状态判断解绑：kubelet在某些情况下会影响Pod Ready的值（[issue #78733](https://github.com/kubernetes/kubernetes/issues/78733))，虽然可以快速回复，但对于一些金融类业务的用户来说，这依然是不可接受的
* 自定义解绑条件：一些K8S平台提供了Pod镜像快速升级的功能，为了进一步减小业务中断时间，Pod在此种情况下不需要在状态变化而解绑。为了将这种快速升级与Pod的异常状态进行区分，这些快速升级技术通常都对Pod进行了特殊处理。
具体设计见文档：[可配置的解绑条件设计](/docs/design/proposal/deregister-policy.md)

### 已实现的业务需求 
业务需求从来不会因K8S的限制而发生改变，这些业务对负载均衡的使用提出了多种多样的需求，这些需求都可以在LBCF的框架内通过自定义逻辑实现。

#### 高度定制化的业务需求
以下需求来自LBCF服务的某**一个**真实业务：
* 每个Pod独占一个LB端口
* 每20个Pod使用一个LB
* Pod扩容时，LB也需要扩容（购买新的LB）
* Pod缩容时，LB也需要缩容（删除LB）
* 所有Pod都来自同一个statefulset，不接受使用多个statefulset的方案
* Pod使用了快速升级技术，期间Pod状态会变为`Pending`，但不允许解绑
* 必须能兼容K8S常规的Pod状态变化（非Ready的Pod需要被解绑）
* LB的接口存在诸多限制，不能同时处理2个请求（调用API时需由调用方加锁）  

#### 对网络有限制的LB
有些LB会对容器网络提出额外需求，需要在注册Pod地址时进行一些定制化配置，由于LBCF是一个纯控制面框架，因此这种LB也可以使用LBCF对接。

## LBCF设计文档

* [LBCF架构设计](/docs/design/lbcf-architecture.md)

* [LBCF CRD设计](/docs/design/lbcf-crd.md)

* [LBCF Webhook规范](/docs/design/lbcf-webhook-specification.md)

* [操作手册](/docs/design/how-to-use.md)

* [一些feature设计](/docs/design/proposal)


本规范定义了Webhook server**必须实现**的8个webhook，其中4种用来操作负载均衡实例，另外4种用来操作被绑定的backend。

| Webhook | 操作对象 | 功能 |
|:---|:---:|:---|
|validateLoadBalancer|LB|验证提交至K8S的LoadBalancer参数的合法性。在创建与更新时都会被调用，可以用来拒绝用户的创建/更新操作|
|createLoadBalancer|LB|创建负载均衡实例|
|ensureLoadBalancer|LB|更新负载均衡实例的配置，有一次性调用与周期性调用两种调用方式|
|deleteLoadBalancer|LB|删除负载均衡实例|
|validateBackend|backend|验证提交至K8S的BackendGroup参数的合法性。在创建与更新时都会被调用，可以用来拒绝用户的创建/更新操作|
|generateBackendAddr|backend|生成绑定backend时使用的backend地址|
|ensureBackend|backend|绑定/更新backend，有一次性调用与周期性调用两种调用方式|
|deregisterBackend|backend|解绑backend|

## webhook的调用时机

**LB相关webhook**

![](media/when-lb-webhooks-are-invoked.png)

**backend相关webhook**

![](media/when-backend-webhooks-are-invoked.png)

## webhook的重试策略

Webhook server在实现上述webhook时无需在本地进行重试，所有重试都由LBCF根据webhook响应按照一定策略自动进行。

LBCF的重试策略分为以下几种：

1. 不重试
    * validateLoadBalancer
    * validateBackend
2. 失败后重试
    * createLoadBalancer
    * ensureLoadBalancer
    * deleteLoadBalancer
    * generateBackendAddr
    * ensureBackend
    * deregisterBackend
3. 周期性调用(需手动开启)
    * ensureLoadBalancer
    * ensureBackend
    
*注：周期性调用的开关仅影响webhook在成功调用后是否依旧被周期性调用，"失败后重试"中的所有webhook都会在失败后被无条件重试*

对于所有"不重试"的webhook，webhook响应中都包含下述统一字段

| Field | Type | Required | Description |
|:---|:---:|:---:|:---|
|succ|bool|TRUE|执行结果|
|msg|string|FALSE|succ为false时需要反馈给用户的信息|

对于所有"失败后重试"和"周期性调用"的webhook，请求与响应中都包含下述共同字段

**公共请求消息体**

| Field | Type | Description |
|:---|:---:|:---|
|recordID|string|任务ID.多次重试间保持不变|
|retryID|string|操作ID.发生重试时会改变|

**公共响应消息体**

| Field | Type | Required | Description |
|:---|:---:|:---:|:---|
|status|string|TRUE|执行结果。支持`Succ`，`Fail`，`Running`，其中Running用来实现异步操作|
|msg|string|FALSE|反馈给用户的信息|
|minRetryDelayinSeconds|string|FALSE|距离下次重试的最小间隔。实际重试间隔受LBCF控制，可能大于此值|

## validateLoadBalancer

## createLoadBalancer

## ensureLoadBalancer

## deleteLoadBalancer

## validateBackend

## generateBackendAddr

## ensureBackend

## deregisterBackend
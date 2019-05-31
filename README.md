## Load Balancer Controlling Framework (LBCF)

LBCF是一款部署在Kubernetes内的通用负载均衡控制面框架，旨在降低容器对接负载均衡的实现难度，并提供强大的扩展能力以满足业务方在使用负载均衡时的个性化需求。

LBCF对K8S内部晦涩的运行机制进行了封装并以Webhook的形式对外暴露，在容器的全生命周期中提供了多达8种Webhook。通过实现这些Webhook，开发人员可以轻松实现下述功能：

* 对接任意负载均衡/名字服务，并自定义对接过程
   
* 实现自定义灰度升级策略

* 容器环境与其他环境共享同一个负载均衡 

* 解耦负载均衡数据面与控制面

## LBCF设计文档

* [LBCF架构设计](docs/design/lbcf-architecture.md)

* [LBCF CRD设计](docs/design/lbcf-crd.md)

* [LBCF Webhook规范](docs/design/lbcf-webhook-specification.md)

* [操作手册](docs/design/how-to-use.md)

## 安装LBCF

系统要求：

* K8S 1.10及以上版本

* 开启Dynamic Admission Control，在apiserver中添加启动参数：
    * --enable-admission-plugins=MutatingAdmissionWebhook,ValidatingAdmissionWebhook

* K8S 1.10版本，在apiserver中额外添加参数：

    * --feature-gates=CustomResourceSubresources=true
    
推荐环境：

在[腾讯云](https://cloud.tencent.com/product/tke)上购买1.12.4版本集群，无需修改任何参数，开箱可用
   

## Examples

* [公有云绑定CLB——Service NodePort方式](docs/examples/tencent-cloud-service-nodeport.md)

* [公有云绑定CLB——Pod直通CLB](docs/examples/tencent-cloud-eni.md)

## 自定义负载均衡对接实践

* [God游戏](https://git.code.oa.com/ianlang/lbcf-driver-ieg-god)
  
  关键词：
  
  * 公有云
  * CLB
  * 游戏
  * 数据面：Pod使用弹性网卡，并直接绑定至CLB（不通过service）
  * 控制面：每个Pod端口独占一个CLB监听器，监听器端口号为`10000 +（pod名后缀[0, n-1] * 10）+ (pod的端口%10)`

  

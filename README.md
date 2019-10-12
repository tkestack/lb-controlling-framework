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

### 系统要求：

* K8S 1.10及以上版本

* 开启Dynamic Admission Control，在apiserver中添加启动参数：
    * --enable-admission-plugins=MutatingAdmissionWebhook,ValidatingAdmissionWebhook

* K8S 1.10版本，在apiserver中额外添加参数：

    * --feature-gates=CustomResourceSubresources=true
    
推荐环境：

在[腾讯云](https://cloud.tencent.com/product/tke)上购买1.12.4版本集群，无需修改任何参数，开箱可用
   

### 步骤1: 制作镜像

进入项目根目录，运行命令制作镜像
```bash
make image
```

### 步骤2：修改YAML中的镜像名称

[deployments目录](deployments)中包含了部署LBCF需要的所有YAML, 在其中找到[deployment.yaml](deployments/deployment.yaml)并使用步骤1生成的镜像替换文件中的`${IMAGE_NAME}`

### 步骤3：安装YAML

登陆K8S集群，使用`kubectl apply -f $file_name` 命令安装[deployments目录](deployments)下的所有YAML文件。

*注：deployments目录中使用的所有证书皆为自签名证书，可按需替换*

## 使用LBCF对接负载均衡/名字服务

LBCF为所有负载均衡提供了统一的控制面，开发人员在对接负载均衡时需要按照[LBCF Webhook规范](docs/design/lbcf-webhook-specification.md)的要求实现Webhook服务器。

Webhook服务器的实现可参考[最佳实践](#best_practice)中的项目。


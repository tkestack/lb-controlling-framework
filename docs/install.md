## 系统要求：

* K8S 1.10及以上版本

* 开启Dynamic Admission Control，在apiserver中添加启动参数：
    * --enable-admission-plugins=MutatingAdmissionWebhook,ValidatingAdmissionWebhook

* K8S 1.10版本，在apiserver中额外添加参数：

    * --feature-gates=CustomResourceSubresources=true
    
推荐环境：

在[腾讯云](https://cloud.tencent.com/product/tke)上购买1.12.4版本集群，无需修改任何参数，开箱可用
   

## 步骤1: 制作镜像

进入项目根目录，运行命令制作镜像
```bash
make image
```

## 步骤2：修改YAML中的镜像名称

[deployments目录](deployments)中包含了部署LBCF需要的所有YAML, 在其中找到[deployment.yaml](/deployments/deployment.yaml)并使用步骤1生成的镜像替换文件中的`${IMAGE_NAME}`

## 步骤3：安装YAML

登陆K8S集群，使用`kubectl apply -f $file_name` 命令安装[deployments目录](/deployments)下的所有YAML文件。

*注：deployments目录中使用的所有证书皆为自签名证书，可按需替换*

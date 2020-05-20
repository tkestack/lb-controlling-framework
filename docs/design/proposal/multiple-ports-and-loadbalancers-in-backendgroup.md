# Background

There has been some complaint about the inconvenience of `BackendGroup`, including:  
* When someone wants to register multiple Pod ports to load balancers, multiple `BackendGroup` must be created because there is only one port can be specified in the `BackendGroup`
* It's not uncommon to register given Pods to multiple load balancers, in such scenario, multiple pairs of `LoadBalancer` and `BackendGroup` must be created

# Solution
1. Substitute `loadBalancers` for `lbName` in `BackendGroup.spec`, backends (Pods/Service NodePort/Static) will be registered to every load balancer in `BackendGroup.spec.loadBalancers`
2. For `BackendGroup` of type `pods`, users may specify multiple ports in `BackendGroup.spec.ports`

# other changes

`portNumber` is deprecated, use `port` instead.

# updated BackendGroup

```yaml
apiVersion: tke.cloud.tencent.com/v1beta1
kind: BackendGroup
metadata: 
  name: my-bg
spec: 
  loadBalancers:
    - my-load-balancer-1 
    - my-load-balancer-2
  pods:
    ports:
      - port: 80
        protocol: TCP
      - port: 90
        protocol: UDP
    byLabel:
      app: my-web-server 
```

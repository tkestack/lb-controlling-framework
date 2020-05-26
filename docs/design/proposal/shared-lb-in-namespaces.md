# Background
It's common that cluster admin creates some load balancers and share them in specific namespaces. In such scenario, 
users that only authorized to specific namespaces are allowed to use the load balancers, but not allowed to create or modify them.

# Solution
We add a `scope` parameter in `LoadBalancer.spec`, it indicates in which namespaces the LoadBalancer is available.
```yaml
apiVersion: lbcf.tkestack.io/v1beta1
kind: LoadBalancer
metadata:
  name: lbcf-my-load-balancer
  namespace: kube-system
spec:
  lbDriver: xxxxx 
  lbSpec:
    lbID: xxxxx
  scope:
  - namespace-a
  - namespace-b
  attributes:
    attr: xxxxx 
```
Specification:  
1. `name` must prefixed with `lbcf-`
2. `namespace` must be `kube-system`
3. `scope` consists of namespaces. `*` indicates all namespaces, including the ones not yet created. If not specifed, 
the LoadBalancer is only available in `kube-system`.
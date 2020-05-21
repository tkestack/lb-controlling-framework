<!-- TOC -->

- [v1.3.0](#v130)
    - [Changelog since v1.2.0](#changelog-since-v120)
        - [Features:](#features)

<!-- /TOC -->

# v1.3.0
## Changelog since v1.2.0

### Features:

* `Added`: two new metrics. [#51e6f](https://github.com/tkestack/lb-controlling-framework/commit/51e6f407e31a1a39e2f8e0e4938b27627c390ea7), [#82c8d](https://github.com/tkestack/lb-controlling-framework/commit/82c8d7da9a0fcfcb81946de972908f06ecd36a38), [proposal](/docs/design/proposal/metrics.md)
* `Added`: deregister policy, users may define a pod deregistering policy other than K8S, which deregisters a pod if it's not ready. [proposal](/docs/design/proposal/deregister-policy.md)
  
  There are 3 available policies in LBCF:
    * `IfNotReady`: The default policy, same as K8S, pods are deregistered if `pod.status.condition[].Ready` is not `True`
    * `IfNotRunning`: Pods are deregistered if `pod.status.phase` is not `Running`. [#618c9](https://github.com/tkestack/lb-controlling-framework/commit/618c9c16414e70107474265ed71120c2fd396abe)
    * `Webhook`: A hightly customizable policy, driver developers may implement their own policy based on Pod. [#3a6c1](https://github.com/tkestack/lb-controlling-framework/commit/3a6c12529d297b7a3f76d9b87b39c5a4de312c72), [#9c037](https://github.com/tkestack/lb-controlling-framework/commit/9c0370290a116ad8c145d65994f57ac170abad37), [#6cd79](https://github.com/tkestack/lb-controlling-framework/commit/6cd7951fadc8a11948b7d68a9a3125c5581111ae), [#ab517](https://github.com/tkestack/lb-controlling-framework/commit/ab517ebecc07ae3e89f0d6cc8f9ab5d57a949873), [#bc406](https://github.com/tkestack/lb-controlling-framework/commit/bc40676818d5ae764c9cb39c7965a896aeebe0d7), [#43751](https://github.com/tkestack/lb-controlling-framework/commit/4375126156f5b54cbd2724936fc10e1962a01e15), [#4ca73](https://github.com/tkestack/lb-controlling-framework/commit/4ca73a2d4416a66ca61a194c2324e05a2d334342)  

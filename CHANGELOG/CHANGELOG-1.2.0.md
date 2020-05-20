# v1.2.0
## Changelog since v1.1.0

### Features:

* `Added`: dry-run mode. [#80d54](https://github.com/tkestack/lb-controlling-framework/commit/80d545843e5353377331d1af7c0aa1f0f2b4caec) [proposal](/docs/design/proposal/dry-run-mode.md)
* `Added`: multiple ports and loadbalancers may be set in one BackendGroup. [#d8624](https://github.com/tkestack/lb-controlling-framework/commit/d86249de75b1d7d5c7d2c36cd7fb9baff1096dd1) [proposal](/docs/design/proposal/multiple-ports-and-loadbalancers-in-backendgroup.md)
* `Added`: exposes some metrics for prometheus. [#400ba](https://github.com/tkestack/lb-controlling-framework/commit/400baf84b81448375632b48d8b077b4a1bd638bc) [proposal](/docs/design/proposal/metrics.md)
* `Deprecated`: `lbName` and `portNumber` in BackendGroup are deprecated, please use `loadBalancers` and `port` instead. [#d8624](https://github.com/tkestack/lb-controlling-framework/commit/d86249de75b1d7d5c7d2c36cd7fb9baff1096dd1) [proposal](/docs/design/proposal/multiple-ports-and-loadbalancers-in-backendgroup.md)

### Other changes:
* `Changed`: BackendGroup status is updated before BackendRecords are created/updated/deleted. [#58af2](https://github.com/tkestack/lb-controlling-framework/commit/58af27c4a58803d484407b60fffbd49a064a419e)
* `Added`: lbcf-controller prints flags being used in bootstrap. [#926d6](https://github.com/tkestack/lb-controlling-framework/commit/926d660f401eccecb937d17bf8ed04572e57398c)
* `Fixed`: lbcf-controller invokes webhook `ensureLoadBalancer` and `ensureBackend` after restarted, even though the `ensurePolicy` is `IfNotSucc`. [#91eea](https://github.com/tkestack/lb-controlling-framework/commit/91eea633c9dcdbaed9f729d28b1fab57f7e504dd)
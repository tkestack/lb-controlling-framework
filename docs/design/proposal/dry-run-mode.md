# Scenario
Updating lbcf-controller or the drivers may cause unexpected behaviors, so the developers want to test their work in a real cluster without modifying it.


In dry-run mode, the following statements are true:

* Drivers may specify if they accept dry-run webhook requests by setting `acceptDryRunCall` in `LoadBalancerDriver`. If true, a `dryRun` flag is set in all webhook requests. Otherwise, no webhooks will be invoked. No matter and `lbcf-controller` treat them as failed.
* `lbcf-controller` treat all dry-run webhook response as failed, so that the status of any CRD is not changed 
* `lbcf-controller` always print webhook names and parameters in log.
* Given an object that `lbcf-controller` is watching, it is processed only once, even after informer resync
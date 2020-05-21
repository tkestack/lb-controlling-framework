# Issues of common controllers
Many controllers are developed based on K8S service controller, these controllers have several issues:  
1. Pods under a service can NOT share the load balancers with servers from other environment. Since the controller keeps comparing endpoints in K8S cluster and servers in the load balancer, it keeps deleting all servers that not in the endpoint list.  
2. One can NOT manipulate the load balancer manually. If the controller makes a mistake, the mistake is kept until the controller is killed, for the controller periodically sync the load balancer servers with endpoints in the K8S cluster.

Some controllers tries to solve above issues by watching and reacting to events, which is unreliable because events in K8S are never re-sent. Once the controller got killed or hangs, the events are lost forever.
    
# Solution of LBCF
There are several goals LBCF aims to achieve:    
1. Pods can share load balacers with other Pods or other servers  
2. Manually manipulating the load balancers should be allowed  
3. The LBCF controller is allowed to be killed at any time  
4. The LBCF controller should make every thing right after it started  

The following figure shows how LBCF do it.

![LBCF keeps pod information in BackendRecords](/docs/design/media/lbcf-consistency.png)  

STAGE 1: LBCF controller create a `BackendRecord` for each `Ready` Pod, all the information that is used to register the pod to the load balancer is stored in the `BackendRecord`. Furthermore, all `BackendRecord`s have `finalizer` set, which makes them can't be deleted before dereigstering is done.  

STAGE 2: Suppose the LBCF controller is killed and Pod 2 was deleted before LBCF controller restarted. Even though LBCF controller will not receive any events that indicate Pod 2 was deleted, it can still realize it by not able to find Pod 2 in the cluster. (LBCF knows there should be Pod 2 because it is recorded in the `BackendGroup`)  

STAGE 3: Once LBCF controller realized Pod 2 was gone, it tries to delete `BR2`. The deleting is blocked, for there is a finalizer on BR 2. LBCF controller watches all `BackendRecord`s, and start the deregistering process once it finds the `deletionTimestamp` is set.  

STAGE 4: Once the deregistering process is finished, LBCF controller delete the finalizer on the `BackendGroup` so that it will be GC by K8S.
# Direct Provider-to-Provider Networking

```{admonition} Note
This feature is experimental. It is currently under testing and may be subject to significant changes.
```

## Overview

In a multi-cluster topology involving a single Consumer cluster and multiple provider clusters, Liqo allows workloads offloaded to different providers to communicate with each other.

![Direct provider-to-provider communication topology](/_static/images/advanced/direct-connection/topology-example.svg)


By default, Liqo routes cross-provider traffic through the Consumer cluster. Traffic originating from a pod on provider A destined for a pod on provider B is first tunneled to the Consumer, and then routed back out to provider B, even in case the inter-cluster networking ([described here](/advanced/peering/inter-cluster-network)) is enabled between the two. 

This feature enables direct east-west traffic between provider clusters. When enabled, Liqo configures the network data plane to allow offloaded pods to communicate directly (provider A ↔ provider B), bypassing the Consumer.

## Prerequisites

- Peering: The Consumer cluster must be correctly peered with at least two provider clusters, as shown in the figure above.

- Inter-provider Connectivity: The inter-cluster networking must be setup between the two providers (easily done with the command `liqoctl network connect` ([check here](/usage/liqoctl/liqoctl_network))).

## Configuration and usage

### Workload deployment
Ensure that a Deployment is active on the Consumer cluster and is successfully offloading pods to multiple Provider clusters. The pods should be distributed across the providers involved in the communication.

### Service exposure
The offloaded pods must be exposed via a standard Kubernetes Service of type `ClusterIP` defined within the Consumer cluster.

### Enabling direct connections
To activate the optimized traffic path, **add this specific annotation to the Service resource**.
- Annotation Key: `use-direct-connections`
- Value: `"true"`


Sample `kubectl` command:

```bash
kubectl annotate service <service-name> -n <namespace_of_the_service> use-direct-connections="true"
```

### Service yaml example

This example shows the yaml that describes the Service exposing a sample `nginx` deployment requiring direct communication.

```yaml
apiVersion: v1
kind: Service
metadata:
  name: nginx-service
  annotations:
    # Enables direct traffic between providers for this service
    use-direct-connections: "true"
spec:
  selector:
    app: nginx-app
    - protocol: TCP
      port: 80
      targetPort: 8080
  type: ClusterIP
```


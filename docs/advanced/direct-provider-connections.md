# Direct connections between providers

## Overview

In a multi-cluster topology, a consumer cluster (C) can offload workloads to multiple provider clusters (P1 and P2) at the same time.
When pods that belong to the same Service are spread across different providers, their traffic must reach each other across cluster boundaries.
By default, this cross-provider communication would have to travel through the consumer cluster — which may be inefficient.

```{figure} /_static/images/advanced/directconnection/docs-directconn.drawio.svg
:align: center
:alt: Direct connections between provider clusters in a multi-provider Liqo topology.
```

**Direct provider connections** let the two provider clusters reach each other's pods directly, through an existing networking peering between them, without involving the consumer cluster as an intermediary.
The feature is opt-in and controlled at the individual Service level: only Services explicitly annotated in the consumer cluster use direct connections, while all other Services continue to work as usual, through the consumer cluster.

No changes are needed in the provider clusters' workloads.

```{admonition} Note
This feature is about *endpoint reachability* for Services. It does not change how pods are scheduled or offloaded across providers. Pods are still placed by the consumer's scheduler on whichever virtual nodes it selects.
```

## Prerequisites

The following must be in place before enabling direct connections for a Service:

1. **Consumer → P1 peering**: the consumer cluster (C) has an active [offloading peering](/usage/peer) with P1. The networking module between C and P1 may be enabled or disabled.
2. **Consumer → P2 peering**: same as above for P2.
3. **Offloaded namespace**: the namespace where the Service and its pods live must be [offloaded](/usage/namespace-offloading) to both P1 and P2, so that workloads can be scheduled on either provider.
4. **P1 → P2 network peering**: P1 and P2 must have an active [networking peering](/advanced/peering/inter-cluster-network) between each other, set up independently from the consumer peerings. Only the networking module is needed; no offloading between P1 and P2 is required.

```{admonition} Note
The network peering between P1 and P2 can be established with [`liqoctl network connect`](/advanced/peering/inter-cluster-network.md#setup-the-inter-cluster-network-via-liqoctl-network-command), which sets up only the networking module without requiring mutual API server access for offloading.
```

## Topology

The simplest topology (picture above) that benefits from this feature involves one consumer and two providers.

A Service may have endpoints (pods) running on both P1 and P2.
When the annotation is present, pods on P1 can reach that Service's endpoints on P2 through the P1↔P2 link, and vice versa — without traffic passing through C.

The feature generalises naturally to any number of providers: as long as a direct network peering exists between each pair of providers, annotating a Service in the consumer cluster enables direct cross-provider communication for all of them.

## Service and EndpointSlice replication

As part of the standard [resource reflection](/usage/reflection.md) process, Liqo automatically replicates into each provider cluster every Service that exists in an offloaded namespace.
This means that a Service created in the consumer cluster (C) is propagated to both P1 and P2, so that pods on either provider can access it by name, exactly as they would with any local Service.

Along with the Service, Liqo reflects the associated `EndpointSlices`: because each provider cluster only sees the pods running locally, it cannot build a complete picture of the Service endpoints on its own.
The consumer's VirtualKubelet fills this gap by propagating the missing entries, with pod IPs remapped to addresses reachable from the destination cluster.

When a Service is annotated to use direct connections, this remapping step is adjusted for its cross-provider endpoints: instead of translating IPs through the consumer's network Configuration, Liqo uses the P1↔P2 network Configuration so that each provider can reach the other's pods through the direct link.
In addition, Liqo keeps a fallback copy of the endpoints reachable through the consumer, so that the Service continues to work even when the direct link is unavailable (see [Failover and fallback](#failover-and-fallback)).

## Enabling direct connections for a Service

To activate this feature for a specific Service, add the following annotation to the **Service resource in the consumer cluster**:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-service
  annotations:
    liqo.io/use-direct-connections: "true"
spec:
  ...
```

When this annotation is present, the Liqo VirtualKubelet in the consumer cluster collects, for each `EndpointSlice` of that Service, the pod IPs belonging to *other* provider clusters (i.e., providers different from the one being reflected to), along with the cluster ID of each pod.
This mapping is embedded as an annotation (`liqo.io/direct-connections-data`) on the `ShadowEndpointSlice` that is sent to each provider.

On the provider side, the `ShadowEndpointSlice` controller reads this data and re-maps those pod IPs using the network Configuration of the direct P1↔P2 peering, instead of using the consumer's network mapping.
This ensures that the `EndpointSlice` entries seen by pods on P1 contain the correctly translated addresses for pods running on P2, and the other way around.

```{admonition} Note
The annotation `liqo.io/direct-connections-data` is internal to Liqo. It is written automatically on `ShadowEndpointSlice` objects and is stripped before the final `EndpointSlice` is written on the provider cluster. You do not need to manage it manually.
```

## Failover and fallback

Direct connections do not make a Service depend on the health of the provider-to-provider link.
If the direct connection between two providers goes down, traffic towards the affected endpoints **automatically fails over** to the standard path through the consumer cluster, and switches back to the direct link once the connection recovers.
No user action is required, and the Service never loses its endpoints.

```{admonition} Note
On the provider clusters, Liqo maintains the fallback endpoints in a companion `EndpointSlice` whose name ends with `-indirect`. Only one of the two slices serves traffic at any time: seeing the other one with endpoints marked not ready is expected, not a symptom of a problem.
```

## Troubleshooting: missing peering between providers

If a Service is annotated with `liqo.io/use-direct-connections: "true"` but no network peering exists between the involved providers (prerequisite 4), the Service **keeps working**: traffic transparently falls back to the path through the consumer cluster.

Liqo signals the misconfiguration with a Warning event (reason: `DirectConnectionNotPeered`) recorded on the reflected Service, which is propagated back to the consumer cluster. You can check for it in both clusters with:

```bash
kubectl describe service my-service -n <namespace>
```

To enable the direct path, establish the missing network peering with [`liqoctl network connect`](/advanced/peering/inter-cluster-network.md#setup-the-inter-cluster-network-via-liqoctl-network-command): the affected Services start using it automatically, with no further action needed.

## Provider-side configuration

Direct connections are initiated by the consumer: it is the consumer that annotates the Service and instructs each provider to route cross-provider traffic through the direct link.
Providers, however, retain full control over whether to comply with this directive.

A provider that does not want to rely on direct connections — for example for security, network segmentation, or trust reasons — can opt out by setting the following Helm value at installation or upgrade time:

```yaml
networking:
  denyDirectConnections: true
```

When this flag is set, the provider never routes traffic through the direct link: annotated Services keep working, but their cross-provider endpoints are always reached through the consumer cluster, exactly as if the annotation were not present.

```{admonition} Note
Denying direct connections does not affect reachability — only the path the traffic takes. If you expect direct-link performance for a Service and observe traffic flowing through the consumer instead, check whether the provider sets this flag.

To fully revert a Service to the standard behavior, remove the `liqo.io/use-direct-connections` annotation from the Service in the consumer cluster.
```


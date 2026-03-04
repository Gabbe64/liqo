# Liqo OpenVPN Feature Branch

This branch provides a reproducible setup to test Liqo peering with an OpenVPN-based gateway.

It is designed for branch developers/testers who need:
- a deterministic Liqo installation path on existing `kind` clusters,
- a clear OpenVPN peering command flow (with `liqoctl`),
- and a small validation workflow (offloading + test workload).


## Prerequisites

- Linux host with Docker working
- `kind`
- `kubectl`
- `liqoctl` (this branch build/binary)
- Two already-created and running `kind` clusters
- Access to two kubeconfig files for those clusters (examples below use `kc1.yaml` and `kc2.yaml`)
- This repository cloned under `$HOME` (installation commands below assume that layout)

### Build local `liqoctl` version

Build `liqoctl` from this source:

```bash
cd "$HOME/liqo"
make ctl
```

This generates `liqoctl` in the same folder (`$HOME/liqo/liqoctl`).


## Install Liqo (custom branch images)

Install Liqo on both clusters with branch-specific controller and OpenVPN gateway images:

I hosted these images in my fork since I lack permissions for the upstream repo containing the official images.
In case you want to build your own images feel free to use the standard building flow.

```bash
./liqoctl install kind --cluster-id cluster1 --cluster-labels=topology.liqo.io/type=origin --kubeconfig kc1.yaml --version v1.0.3 --local-chart-path "$HOME/liqo/deployments/liqo/" \
  --set controllerManager.image.name=ghcr.io/gabbe64/liqo-controller-manager \
  --set controllerManager.image.version=openvpn \
  --set networking.gatewayTemplates.container.openvpn.image.name=ghcr.io/gabbe64/liqo-openvpn \
  --set networking.gatewayTemplates.container.openvpn.image.version=latest

./liqoctl install kind --cluster-id cluster2 --cluster-labels=topology.liqo.io/type=destination --kubeconfig kc2.yaml --version v1.0.3 --local-chart-path "$HOME/liqo/deployments/liqo/" \
  --set controllerManager.image.name=ghcr.io/gabbe64/liqo-controller-manager \
  --set controllerManager.image.version=openvpn \
  --set networking.gatewayTemplates.container.openvpn.image.name=ghcr.io/gabbe64/liqo-openvpn \
  --set networking.gatewayTemplates.container.openvpn.image.version=latest
```
## Peer clusters with OpenVPN gateway

Run peering from cluster1 to cluster2 using OpenVPN tunneling and `NodePort` service exposure:

```bash
./liqoctl peer --tunneling-protocol openvpn --kubeconfig kc1.yaml --remote-kubeconfig kc2.yaml --gw-server-service-type NodePort
```

## Validate offloading and connectivity

Create and offload a namespace from cluster1:

```bash
kubectl create namespace testing --kubeconfig kc1.yaml
./liqoctl offload namespace testing --namespace-mapping-strategy EnforceSameName --pod-offloading-strategy LocalAndRemote --kubeconfig kc1.yaml
```

Deploy a sample app:

```bash
kubectl create deployment nginx --image=nginx -n testing --kubeconfig kc1.yaml
kubectl expose deployment nginx -n testing --type=ClusterIP --port=80 --target-port=80 --kubeconfig kc1.yaml
```

Now offloading is active on the `testing` namespace.

## OpenVPN gateway template args customization

When Liqo is installed, it creates two template resources used for OpenVPN peering:
- `OvpnGatewayServerTemplate` (server side)
- `OvpnGatewayClientTemplate` (client side)

To change OpenVPN runtime behavior, edit the `openvpn` container `args` inside those templates.
The field to edit is:

`spec.template.spec.deployment.spec.template.spec.containers[]` (the item with `name: openvpn`).

### Workflow

Export the current templates:

```bash
kubectl get ovpngatewayservertemplate openvpn-gwserver -n liqo -o yaml > ovpn-gwserver-template.yaml
kubectl get ovpngatewayclienttemplate openvpn-gwclient -n liqo -o yaml > ovpn-gwclient-template.yaml
```

Edit the `args` list of the `openvpn` container in both files. Example of the section to tweak:

```yaml
spec:
  template:
    spec:
      deployment:
        spec:
          template:
            spec:
              containers:
              - name: openvpn
                args:
                - --mode=server
                - --ifconfig-local-ip=169.254.18.1
                - --ifconfig-remote-ip=169.254.18.2
                - --port={{ .Spec.Endpoint.Port }}
                # add/change additional args supported by your OpenVPN container
```

Apply the updated templates:

```bash
kubectl apply -f ovpn-gwserver-template.yaml
kubectl apply -f ovpn-gwclient-template.yaml
```

Notes:
- keep required placeholders intact when present (e.g. `{{ .Spec.Endpoint.Port }}`);
- keep role-specific modes coherent (`server` on server template, `client` on client template);
- template updates are applied to new gateway resources, so recreate/re-peer if existing gateways were already generated.

## References in this repo

- `liqo/docs/usage/liqoctl/liqoctl_peer.md`
- `liqo/docs/advanced/nat.md`

## Experimental status

This work is an experimental feature developed as a proof of concept.

It is intended to:
- validate the OpenVPN tunneling path in Liqo,
- provide a practical reference for testing and iteration,
- and act as a guideline for implementing and integrating new tunneling features in the future.

It should be treated as a development/testing artifact, not as a production-ready reference.


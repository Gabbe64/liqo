package virtualkubelet

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	fccApi "github.com/Gabbe64/foreign_cluster_connector/api/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Contains both the CIDR to use when checking shortcut presence (PodCIDR) and the CIDR to use when remapping the address (ShortcutPodCIDR).
// 
// Precisely, PodCIDR is the CIDR of the cluster at the other end of the shortcut SEEN BY THE CENTRAL ONE.
//
// 
// Example: if B and C are connected by a shortcut and A is central cluster then: 
// 
// VK reflecting to B will have the PodCIDR of HOW A SEES C
//
// VK reflecting to C will have the PodCIDR of HOW A SEES B 
type CidrInfo struct {
    PodCIDR string
    ShortcutPodCIDR string
}

// Creates a client and fetches a ForeignClusterConnectionList (which contains all the ForeignClusterConnection) in the namespace.
func ListForeignClusterConnections(namespace string, ctx context.Context) (*fccApi.ForeignClusterConnectionList, error) {
    // Use in-cluster config or kubeconfig as appropriate
    cfg, err := rest.InClusterConfig() // or clientcmd.BuildConfigFromFlags("", kubeconfigPath)
    if err != nil {
        return nil, err
    }

	scheme := runtime.NewScheme()
	_ = fccApi.AddToScheme(scheme) // Register the ForeignClusterConnection API scheme 

    c, err := client.New(cfg, client.Options{Scheme: scheme})
    if err != nil {
        return nil, err
    }


    var fcclist fccApi.ForeignClusterConnectionList
    //key := client.ObjectKey{Namespace: namespace, Name: name}
    if err := c.List(ctx, &fcclist); err != nil {
        return nil, err
    }
    return &fcclist, nil
}


func GetForeignClusterConnectionsCIDRs(list *fccApi.ForeignClusterConnectionList) ([]string, error) {
	if list == nil || len(list.Items) == 0 {
		return nil, fmt.Errorf("no ForeignClusterConnections found")
	}

	cidrs := make([]string, 0, len(list.Items))
	for _, fcc := range list.Items {
		if fcc.Status.ForeignClusterANetworking.PodCIDR != "" {
			cidrs = append(cidrs, fcc.Status.ForeignClusterANetworking.PodCIDR)
		} else {
			klog.Warningf("ForeignClusterConnection %s has no CIDR specified", fcc.Name)
		}
	}

	if len(cidrs) == 0 {
		return nil, fmt.Errorf("no valid CIDRs found in ForeignClusterConnections")
	}

	return cidrs, nil
}

// Returns all the CIDRs of the shortcuts using the cluster name (that is, the shortcuts involving that cluster) and the ForeignClusterConnectionList.
// These CIDRs represent the subnets of the clusters that must be changed in order to leverage the shortcuts.
func GetAllCidrsByClusterName(list *fccApi.ForeignClusterConnectionList, clusterName string) ([]CidrInfo, error) {
    if list == nil || len(list.Items) == 0 {
        return nil, fmt.Errorf("no ForeignClusterConnections found")
    }

    var cidrs []CidrInfo
    for _, fcc := range list.Items {
        if fcc.Spec.ForeignClusterA == clusterName {
            if fcc.Status.ForeignClusterANetworking.PodCIDR != "" {
                c := CidrInfo{
                    PodCIDR: fcc.Status.ForeignClusterANetworking.PodCIDR,
                    ShortcutPodCIDR: fcc.Status.ForeignClusterANetworking.RemappedPodCIDR,
                }
                cidrs = append(cidrs, c)
                break // Assuming that if matches with A it won't match with B
            }
        }
        if fcc.Spec.ForeignClusterB == clusterName {
            if fcc.Status.ForeignClusterBNetworking.PodCIDR != "" {
                c := CidrInfo{
                    PodCIDR: fcc.Status.ForeignClusterBNetworking.PodCIDR,
                    ShortcutPodCIDR: fcc.Status.ForeignClusterBNetworking.RemappedPodCIDR,
                }
                cidrs = append(cidrs, c)
            }
        }
    }

    if len(cidrs) == 0 {
        return nil, fmt.Errorf("no valid CIDRs found for cluster name: %s", clusterName)
    }

    return cidrs, nil
}

// GetCidrByClusterName returns the CIDR of the shortcut using the cluster name and the ForeignClusterConnectionList.
func GetCidrByClusterName(list *fccApi.ForeignClusterConnectionList, clusterName string) (string, error) {
    if list == nil || len(list.Items) == 0 {
        return "", fmt.Errorf("no ForeignClusterConnections found")
    }

    for _, fcc := range list.Items {
        if fcc.Spec.ForeignClusterA == clusterName {
            if fcc.Status.ForeignClusterANetworking.PodCIDR != "" {
                return fcc.Status.ForeignClusterANetworking.PodCIDR, nil
            }
        }
    }

    return "", fmt.Errorf("no ForeignClusterConnection found for cluster name: %s", clusterName)
}

// IpBelongsToCIDR checks if the given IP address belongs to the specified CIDR.
func IpBelongsToCIDR(ipAddr string, cidrStr string) (bool, error) {
    // Parse the IP address
    ip := net.ParseIP(ipAddr)
    if ip == nil {
        return false, fmt.Errorf("invalid IP address: %s", ipAddr)
    }
    
    // Parse the CIDR
    _, cidr, err := net.ParseCIDR(cidrStr)
    if err != nil {
        return false, fmt.Errorf("invalid CIDR: %s - %w", cidrStr, err)
    }
    
    // Check if the IP is in the CIDR range
    return cidr.Contains(ip), nil
}

// Reads the environment variable POD_NAMESPACE to get the cluster name.
func GetClusterNameFromNamespace() string {
	podNamespace := os.Getenv("POD_NAMESPACE")

	// Check if the namespace starts with the expected prefix
    const prefix = "liqo-tenant-"
    if podNamespace != "" && len(podNamespace) > len(prefix) && strings.HasPrefix(podNamespace, prefix) {
        // Extract the part after the prefix
        clusterName := podNamespace[len(prefix):]
        return clusterName
    }
    
    // Return empty string or default if prefix not found
    klog.Warningf("Could not extract cluster name from namespace: %s", podNamespace)
    return ""
}

// Remaps the address using the provided cidr.
func RemapAddressUsingCidr(address string, cidr string) ([]string, error) {
    ip := net.ParseIP(address)
    if ip == nil {
        return nil, fmt.Errorf("invalid IP address: %s", address)
    }

    _, ipNet, err := net.ParseCIDR(cidr)
    if err != nil {
        return nil, fmt.Errorf("invalid CIDR: %s - %w", cidr, err)
    }

    ip = ip.To4()
    if ip == nil {
        return nil, fmt.Errorf("only IPv4 addresses are supported")
    }
    network := ipNet.IP.To4()
    if network == nil {
        return nil, fmt.Errorf("only IPv4 CIDRs are supported")
    }

    mask := ipNet.Mask
    remapped := make(net.IP, 4)
    for i := 0; i < 4; i++ {
        remapped[i] = (network[i] & mask[i]) | (ip[i] & ^mask[i])
    }

    return []string{remapped.String()}, nil
}
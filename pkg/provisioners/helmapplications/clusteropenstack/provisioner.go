/*
Copyright 2022-2024 EscherCloud.
Copyright 2024 the Unikorn Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package clusteropenstack

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/unikorn-cloud/core/pkg/constants"
	"github.com/unikorn-cloud/core/pkg/provisioners/application"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	kubernetesprovisioners "github.com/unikorn-cloud/kubernetes/pkg/provisioners"
)

// Provisioner encapsulates control plane provisioning.
type Provisioner struct {
	options *kubernetesprovisioners.ClusterOpenstackOptions
	// clusterManagerPrefix contains the IP address prefix to add
	// to the cluster firewall if, required.
	clusterManagerPrefix string
}

// New returns a new initialized provisioner object.
func New(getApplication application.GetterFunc, options *kubernetesprovisioners.ClusterOpenstackOptions, clusterManagerPrefix string) *application.Provisioner {
	provisioner := &Provisioner{
		options:              options,
		clusterManagerPrefix: clusterManagerPrefix,
	}

	return application.New(getApplication).WithGenerator(provisioner).AllowDegraded()
}

// Ensure the Provisioner interface is implemented.
var _ application.ReleaseNamer = &Provisioner{}
var _ application.ValuesGenerator = &Provisioner{}
var _ application.PostProvisionHook = &Provisioner{}

// generateMachineHelmValues translates the API's idea of a machine into what's
// expected by the underlying Helm chart.
func (p *Provisioner) generateMachineHelmValues(machine *unikornv1.MachineGeneric, controlPlane bool) map[string]interface{} {
	object := map[string]interface{}{
		"imageID":  *machine.ImageID,
		"flavorID": *machine.FlavorName,
	}

	if controlPlane && p.options.ServerGroupID != nil {
		object["serverGroupID"] = *p.options.ServerGroupID
	}

	if machine.DiskSize != nil {
		disk := map[string]interface{}{
			"size": machine.DiskSize.Value() >> 30,
		}

		object["disk"] = disk
	}

	return object
}

// generateWorkloadPoolHelmValues translates the API's idea of a workload pool into
// what's expected by the underlying Helm chart.
func (p *Provisioner) generateWorkloadPoolHelmValues(cluster *unikornv1.KubernetesCluster) map[string]interface{} {
	workloadPools := map[string]interface{}{}

	for i := range cluster.Spec.WorkloadPools.Pools {
		workloadPool := &cluster.Spec.WorkloadPools.Pools[i]

		object := map[string]interface{}{
			"replicas": *workloadPool.Replicas,
			"machine":  p.generateMachineHelmValues(&workloadPool.MachineGeneric, false),
		}

		if cluster.AutoscalingEnabled() && workloadPool.Autoscaling != nil {
			object["autoscaling"] = generateWorkloadPoolSchedulerHelmValues(workloadPool)
		}

		if len(workloadPool.Labels) != 0 {
			labels := map[string]interface{}{}

			for key, value := range workloadPool.Labels {
				labels[key] = value
			}

			object["labels"] = labels
		}

		if len(workloadPool.Files) != 0 {
			files := make([]interface{}, len(workloadPool.Files))

			for i, file := range workloadPool.Files {
				files[i] = map[string]interface{}{
					"path":    *file.Path,
					"content": base64.StdEncoding.EncodeToString(file.Content),
				}
			}

			object["files"] = files
		}

		workloadPools[workloadPool.Name] = object
	}

	return workloadPools
}

// generateWorkloadPoolSchedulerHelmValues translates from Kubernetes API scheduling
// parameters into ones acceptable by Helm.
func generateWorkloadPoolSchedulerHelmValues(p *unikornv1.KubernetesClusterWorkloadPoolsPoolSpec) map[string]interface{} {
	// When enabled, scaling limits are required.
	values := map[string]interface{}{
		"limits": map[string]interface{}{
			"minReplicas": *p.Autoscaling.MinimumReplicas,
			"maxReplicas": *p.Autoscaling.MaximumReplicas,
		},
	}

	// When scaler from zero is enabled, you need to provide CPU and memory hints,
	// the autoscaler cannot guess the flavor attributes.
	if p.Autoscaling.Scheduler != nil {
		scheduling := map[string]interface{}{
			"cpu":    *p.Autoscaling.Scheduler.CPU,
			"memory": fmt.Sprintf("%dG", p.Autoscaling.Scheduler.Memory.Value()>>30),
		}

		// If the flavor has a GPU, then we also need to inform the autoscaler
		// about the GPU scheduling information.
		if p.Autoscaling.Scheduler.GPU != nil {
			gpu := map[string]interface{}{
				"type":  *p.Autoscaling.Scheduler.GPU.Type,
				"count": *p.Autoscaling.Scheduler.GPU.Count,
			}

			scheduling["gpu"] = gpu
		}

		values["scheduler"] = scheduling
	}

	return values
}

func (p *Provisioner) generateNetworkValues(cluster *unikornv1.KubernetesCluster) interface{} {
	nameservers := make([]interface{}, len(cluster.Spec.Network.DNSNameservers))

	for i, nameserver := range cluster.Spec.Network.DNSNameservers {
		nameservers[i] = nameserver.IP.String()
	}

	values := map[string]interface{}{
		"nodeCIDR": cluster.Spec.Network.NodeNetwork.IPNet.String(),
		"serviceCIDRs": []interface{}{
			cluster.Spec.Network.ServiceNetwork.IPNet.String(),
		},
		"podCIDRs": []interface{}{
			cluster.Spec.Network.PodNetwork.IPNet.String(),
		},
		"dnsNameservers": nameservers,
	}

	if p.options.ProviderNetwork != nil {
		values["provider"] = map[string]interface{}{
			"networkID": *p.options.ProviderNetwork.NetworkID,
			"subnetID":  *p.options.ProviderNetwork.SubnetID,
		}
	}

	if p.options.SSHKeyName != nil {
		values["securityGroupRules"] = []interface{}{
			map[string]interface{}{
				"name":         "ssh-ingress",
				"direction":    "ingress",
				"etherType":    "IPv4",
				"protocol":     "TCP",
				"portRangeMin": 22,
				"portRangeMax": 22,
			},
		}
	}

	return values
}

// Generate implements the application.Generator interface.
func (p *Provisioner) Values(ctx context.Context, version *string) (interface{}, error) {
	//nolint:forcetypeassert
	cluster := application.FromContext(ctx).(*unikornv1.KubernetesCluster)

	workloadPools := p.generateWorkloadPoolHelmValues(cluster)

	openstackValues := map[string]interface{}{
		"cloud":      p.options.Cloud,
		"cloudsYAML": p.options.CloudConfig,
	}

	if p.options.ExternalNetworkID != nil {
		openstackValues["externalNetworkID"] = *p.options.ExternalNetworkID
	}

	if p.options.SSHKeyName != nil {
		openstackValues["sshKeyName"] = *p.options.SSHKeyName
	}

	labels, err := cluster.ResourceLabels()
	if err != nil {
		return nil, err
	}

	// These must have parity with what's defined by the API to make
	// cross referencing between unikorn and openstack logging easier.
	// TODO: clusterID is not going to cut it moving forward, especially
	// when baremetal clusters are a thing and we'll need to differentiate
	// between them.
	serverMetadata := map[string]interface{}{
		"clusterID":      cluster.Name,
		"projectID":      labels[constants.ProjectLabel],
		"organizationID": labels[constants.OrganizationLabel],
	}

	// TODO: generate types from the Helm values schema.
	values := map[string]interface{}{
		"version":   string(*cluster.Spec.Version),
		"openstack": openstackValues,
		"cluster": map[string]interface{}{
			"taints": []interface{}{
				// TODO: This is deprecated moving forward as the cilium operator provides these taints.
				//   We can't remove it yet though as it'd break any deployments pre kubernetes-cluster-1.4.0

				// This prevents things like coreDNS from coming up until
				// the CNI is installed.
				map[string]interface{}{
					"key":    "node.cilium.io/agent-not-ready",
					"effect": "NoSchedule",
					"value":  "true",
				},
			},
			"serverMetadata": serverMetadata,
		},
		"controlPlane": map[string]interface{}{
			"replicas": *cluster.Spec.ControlPlane.Replicas,
			"machine":  p.generateMachineHelmValues(&cluster.Spec.ControlPlane.MachineGeneric, true),
		},
		"workloadPools": workloadPools,
		"network":       p.generateNetworkValues(cluster),
	}

	if cluster.Spec.API != nil {
		apiValues := map[string]interface{}{}

		if cluster.Spec.API.SubjectAlternativeNames != nil {
			apiValues["certificateSANs"] = cluster.Spec.API.SubjectAlternativeNames
		}

		if cluster.Spec.API.AllowedPrefixes != nil {
			// Add the SNAT IP so CAPI can manage the cluster.
			allowList := []interface{}{
				p.clusterManagerPrefix,
			}

			for _, prefix := range cluster.Spec.API.AllowedPrefixes {
				allowList = append(allowList, prefix.IPNet.String())
			}

			apiValues["allowList"] = allowList
		}

		values["api"] = apiValues
	}

	return values, nil
}

// ReleaseName implements the application.ReleaseNamer interface.
func (p *Provisioner) ReleaseName(ctx context.Context) string {
	//nolint:forcetypeassert
	cluster := application.FromContext(ctx).(*unikornv1.KubernetesCluster)

	return releaseName(cluster)
}

// PostHook implements the apllication PostProvisionHook interface.
func (p *Provisioner) PostProvision(ctx context.Context) error {
	return p.deleteOrphanedMachineDeployments(ctx)
}

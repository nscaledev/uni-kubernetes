/*
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

package provisioners

// ClusterOpenstackOptions are acquired from the region controller at
// reconcile time as the identity provisioning is asynchronous.
type ClusterOpenstackOptions struct {
	// CloudConfig is a base64 encoded minimal clouds.yaml file for
	// use by the ClusterManager to provision the IaaS bits.
	CloudConfig string
	// Cloud is the clouds.yaml key that identifes the configuration
	// to use for provisioning.
	Cloud string
	// ExternalNetworkID is the Openstack external network ID.
	ExternalNetworkID *string
	// ProviderNetwork is the provider network to pass to the cluster
	// provisioner.
	ProviderNetwork *ClusterOpenstackProviderOptions
	// ServerGroupID is the server group used for HA control planes.
	ServerGroupID *string
}

type ClusterOpenstackProviderOptions struct {
	// NetworkID is the network to use for provisioning the cluster on.
	// This is typically used to pass in bare-metal provider networks.
	NetworkID *string
	// SubnetID is the subnet to use for provisioning the cluster on.
	// This is typically used to pass in bare-metal provider networks.
	SubnetID *string
}

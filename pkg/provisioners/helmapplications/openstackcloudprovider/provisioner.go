/*
Copyright 2022-2024 EscherCloud.
Copyright 2024-2025 the Unikorn Authors.

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

package openstackcloudprovider

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"

	"github.com/gophercloud/utils/openstack/clientconfig"
	ini "gopkg.in/ini.v1"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	coreclient "github.com/unikorn-cloud/core/pkg/client"
	"github.com/unikorn-cloud/core/pkg/constants"
	"github.com/unikorn-cloud/core/pkg/provisioners"
	"github.com/unikorn-cloud/core/pkg/provisioners/application"
	"github.com/unikorn-cloud/core/pkg/provisioners/util"
	kubernetesprovisioners "github.com/unikorn-cloud/kubernetes/pkg/provisioners"

	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

var (
	// ErrCloudConfiguration is returned when the cloud configuration is not
	// correctly formatted.
	ErrCloudConfiguration = errors.New("invalid cloud configuration")
)

// Provisioner encapsulates control plane provisioning.
type Provisioner struct {
	options *kubernetesprovisioners.ClusterOpenstackOptions
}

var _ application.PreDeprovisionHook = &Provisioner{}

// New returns a new initialized provisioner object.
func New(getApplication application.GetterFunc, options *kubernetesprovisioners.ClusterOpenstackOptions) *application.Provisioner {
	provisioner := &Provisioner{
		options: options,
	}

	return application.New(getApplication).WithGenerator(provisioner)
}

// Ensure the Provisioner interface is implemented.
var _ application.ValuesGenerator = &Provisioner{}

// GenerateCloudConfig does the horrific translation between the myriad ways that OpenStack
// deems necessary to authenticate to the cloud configuration format.  See:
// https://github.com/kubernetes/cloud-provider-openstack/blob/master/docs/openstack-cloud-controller-manager/using-openstack-cloud-controller-manager.md#config-openstack-cloud-controller-manager
//
//nolint:cyclop
func GenerateCloudConfig(options *kubernetesprovisioners.ClusterOpenstackOptions) (string, error) {
	cloudConfigYAML, err := base64.URLEncoding.DecodeString(options.CloudConfig)
	if err != nil {
		return "", err
	}

	var clouds clientconfig.Clouds

	if err := yaml.Unmarshal(cloudConfigYAML, &clouds); err != nil {
		return "", err
	}

	cloud, ok := clouds.Clouds[options.Cloud]
	if !ok {
		return "", fmt.Errorf("%w: cloud '%s' not found in clouds.yaml", ErrCloudConfiguration, options.Cloud)
	}

	if cloud.AuthType != clientconfig.AuthV3ApplicationCredential {
		return "", fmt.Errorf("%w: v3applicationcredential auth_type must be specified in clouds.yaml", ErrCloudConfiguration)
	}

	cloudConfig := ini.Empty()

	global, err := cloudConfig.NewSection("Global")
	if err != nil {
		return "", err
	}

	if _, err := global.NewKey("auth-url", cloud.AuthInfo.AuthURL); err != nil {
		return "", err
	}

	if _, err := global.NewKey("application-credential-id", cloud.AuthInfo.ApplicationCredentialID); err != nil {
		return "", err
	}

	if _, err := global.NewKey("application-credential-secret", cloud.AuthInfo.ApplicationCredentialSecret); err != nil {
		return "", err
	}

	loadBalancer, err := cloudConfig.NewSection("LoadBalancer")
	if err != nil {
		return "", err
	}

	if _, err := loadBalancer.NewKey("floating-network-id", *options.ExternalNetworkID); err != nil {
		return "", err
	}

	if _, err := loadBalancer.NewKey("create-monitor", "true"); err != nil {
		return "", err
	}

	blockStorage, err := cloudConfig.NewSection("BlockStorage")
	if err != nil {
		return "", err
	}

	if _, err := blockStorage.NewKey("ignore-volume-az", "true"); err != nil {
		return "", err
	}

	buffer := &bytes.Buffer{}

	if _, err := cloudConfig.WriteTo(buffer); err != nil {
		return "", err
	}

	return buffer.String(), nil
}

// Generate implements the application.Generator interface.
// Note there is an option, to just pass through the clouds.yaml file, however
// the chart doesn't allow it to be exposed so we need to translate between formats.
func (p *Provisioner) Values(ctx context.Context, _ unikornv1core.SemanticVersion) (any, error) {
	cloudConfig, err := GenerateCloudConfig(p.options)
	if err != nil {
		return nil, err
	}

	cloudConfigHash, err := util.GetConfigurationHash(cloudConfig)
	if err != nil {
		return nil, err
	}

	tolerations := util.ControlPlaneTolerations()
	tolerations = append(tolerations, util.ControlPlaneInitTolerations()...)

	values := map[string]any{
		"commonAnnotations": map[string]any{
			constants.ConfigurationHashAnnotation: cloudConfigHash,
		},
		"cloudConfigContents": cloudConfig,
		"tolerations":         tolerations,
		// See https://github.com/kubernetes/cloud-provider-openstack/issues/2049 for
		// more details, and no-one doing anything about it.
		"controllerExtraArgs": `{{list "--use-service-account-credentials=false" | toYaml}}`,
		// See https://github.com/kubernetes/cloud-provider-openstack/issues/2611
		"dnsPolicy": "Default",
	}

	return values, nil
}

// PreDeprovision allows us to delete all load balancer resources that are in
// existence and free them, and their IP addresses up, as some cloud platforms
// will not do this for themselves.
func (p *Provisioner) PreDeprovision(ctx context.Context) error {
	log := log.FromContext(ctx)

	clusterContext, err := coreclient.ClusterFromContext(ctx)
	if err != nil {
		return err
	}

	client := clusterContext.Client

	var services corev1.ServiceList

	if err := client.List(ctx, &services); err != nil {
		return err
	}

	services.Items = slices.DeleteFunc(services.Items, func(service corev1.Service) bool {
		return service.Spec.Type != corev1.ServiceTypeLoadBalancer
	})

	log.Info("freeing load balancer services before removing cloud controller", "remanaing", len(services.Items))

	if len(services.Items) == 0 {
		return nil
	}

	for i := range services.Items {
		if err := client.Delete(ctx, &services.Items[i]); err != nil {
			return err
		}
	}

	return provisioners.ErrYield
}

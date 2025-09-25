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

package openstackplugincindercsi

import (
	"context"
	"strings"

	unikornv1core "github.com/unikorn-cloud/core/pkg/apis/unikorn/v1alpha1"
	coreclient "github.com/unikorn-cloud/core/pkg/client"
	"github.com/unikorn-cloud/core/pkg/constants"
	"github.com/unikorn-cloud/core/pkg/provisioners/application"
	"github.com/unikorn-cloud/core/pkg/provisioners/util"
	kubernetesprovisioners "github.com/unikorn-cloud/kubernetes/pkg/provisioners"
	"github.com/unikorn-cloud/kubernetes/pkg/provisioners/helmapplications/openstackcloudprovider"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/yaml"
)

// Provisioner provides helm configuration interfaces.
type Provisioner struct {
	options *kubernetesprovisioners.ClusterOpenstackOptions
}

// New returns a new initialized provisioner object.
func New(getApplication application.GetterFunc, options *kubernetesprovisioners.ClusterOpenstackOptions) *application.Provisioner {
	provisioner := &Provisioner{
		options: options,
	}

	return application.New(getApplication).WithGenerator(provisioner)
}

// Ensure the Provisioner interface is implemented.
var _ application.ValuesGenerator = &Provisioner{}

func (p *Provisioner) generateStorageClass(name string, reclaimPolicy corev1.PersistentVolumeReclaimPolicy, volumeBindingMode storagev1.VolumeBindingMode, isDefault, volumeExpansion bool) *storagev1.StorageClass {
	class := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner:          "cinder.csi.openstack.org",
		Parameters:           map[string]string{},
		ReclaimPolicy:        &reclaimPolicy,
		AllowVolumeExpansion: &volumeExpansion,
		VolumeBindingMode:    &volumeBindingMode,
	}

	if isDefault {
		class.Annotations = map[string]string{
			"storageclass.kubernetes.io/is-default-class": "true",
		}
	}

	return class
}

func (p *Provisioner) generateStorageClasses() []*storagev1.StorageClass {
	return []*storagev1.StorageClass{
		p.generateStorageClass("cinder", corev1.PersistentVolumeReclaimDelete, storagev1.VolumeBindingWaitForFirstConsumer, true, true),
	}
}

// Generate implements the application.ValuesGenerator interface.
func (p *Provisioner) Values(ctx context.Context, version unikornv1core.SemanticVersion) (any, error) {
	client, err := coreclient.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	storageClasses := p.generateStorageClasses()

	yamls := make([]string, len(storageClasses))

	for i, class := range storageClasses {
		var u unstructured.Unstructured

		// While we could hard code the TypeMeta, it's better practice and safer
		// to use the provided mechanisms.
		if err := client.Scheme().Convert(class, &u, nil); err != nil {
			return nil, err
		}

		y, err := yaml.Marshal(u.Object)
		if err != nil {
			return nil, err
		}

		yamls[i] = string(y)
	}

	cloudConfig, err := openstackcloudprovider.GenerateCloudConfig(p.options)
	if err != nil {
		return nil, err
	}

	cloudConfigHash, err := util.GetConfigurationHash(cloudConfig)
	if err != nil {
		return nil, err
	}

	values := map[string]any{
		"commonAnnotations": map[string]any{
			constants.ConfigurationHashAnnotation: cloudConfigHash,
		},
		// Allow scale to zero.
		"csi": map[string]any{
			"plugin": map[string]any{
				"controllerPlugin": map[string]any{
					"nodeSelector": util.ControlPlaneNodeSelector(),
					"tolerations":  util.ControlPlaneTolerations(),
				},
			},
		},
		"storageClass": map[string]any{
			"enabled": false,
			"custom":  strings.Join(yamls, "---\n"),
		},
	}

	return values, nil
}

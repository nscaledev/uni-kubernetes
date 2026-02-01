/*
Copyright 2022-2024 EscherCloud.
Copyright 2024-2025 the Unikorn Authors.
Copyright 2026 Nscale.

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

package vcluster

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	coreclient "github.com/unikorn-cloud/core/pkg/client"
	"github.com/unikorn-cloud/core/pkg/provisioners"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	ErrConfigDataMissing = errors.New("config data not found")
)

// releaseName generates a unique helm-compliant release name from the cluster's
// name.
func releaseName(name string) string {
	// This must be no longer than 53 characters and unique across all control
	// planes to avoid OpenStack network aliasing.
	sum := sha256.Sum256([]byte(name))

	hash := fmt.Sprintf("%x", sum)

	return "vcluster-" + hash[:8]
}

// VCluster provides services around virtual clusters.
type VCluster interface {
	// ClientConfig returns a raw client configuration.
	ClientConfig(ctx context.Context, namespace, name string, external bool) (*clientcmdapi.Config, error)

	// RESTConfig returns a REST client configuration.
	RESTConfig(ctx context.Context, namespace, name string, external bool) (*rest.Config, error)
}

// ConfigGetter abstracts the fact that we call this code from a controller-runtime
// world, and a kubectl one, each having wildly different client models.
type ConfigGetter interface {
	// GetSecret provides an implementation specific way to get a secret.
	GetSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error)
}

// ControllerRuntimeClient provides vcluster services for controllers.
type ControllerRuntimeClient struct{}

// NewControllerRuntimeClient returns vcluster abstraction with a controller
// runtime client.
func NewControllerRuntimeClient() *ControllerRuntimeClient {
	return &ControllerRuntimeClient{}
}

// Ensure all the interfaces are correctly implemented.
var _ VCluster = &ControllerRuntimeClient{}
var _ ConfigGetter = &ControllerRuntimeClient{}

// ClientConfig returns a raw client configuration.
func (c *ControllerRuntimeClient) ClientConfig(ctx context.Context, namespace, name string, external bool) (*clientcmdapi.Config, error) {
	return getClientConfig(ctx, c, namespace, name, external)
}

// RESTConfig returns a REST client configuration.
func (c *ControllerRuntimeClient) RESTConfig(ctx context.Context, namespace, name string, external bool) (*rest.Config, error) {
	getter := func() (*clientcmdapi.Config, error) {
		return c.ClientConfig(ctx, namespace, name, external)
	}

	return clientcmd.BuildConfigFromKubeconfigGetter("", getter)
}

// GetSecret provides an implementation specific way to get a secret.
func (c *ControllerRuntimeClient) GetSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	log := log.FromContext(ctx)

	clusterContext, err := coreclient.ClusterFromContext(ctx)
	if err != nil {
		return nil, err
	}

	secret := &corev1.Secret{}
	if err := clusterContext.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, secret); err != nil {
		if kerrors.IsNotFound(err) {
			log.Info("vitual cluster kubeconfig does not exist, yielding")

			return nil, provisioners.ErrYield
		}

		return nil, err
	}

	return secret, nil
}

// Client returns a controller runtime client able to access resources in the vcluster.
func (c *ControllerRuntimeClient) Client(ctx context.Context, namespace, name string, external bool) (client.Client, error) {
	config, err := c.RESTConfig(ctx, namespace, name, external)
	if err != nil {
		return nil, err
	}

	return client.New(config, client.Options{})
}

// getClientConfig acknowledges that vcluster configuration is synchronized by a side car, so it
// performs a retry until the provided context expires.  It also acknowledges that load
// balancer services may take a while to get a public IP.
func getClientConfig(ctx context.Context, getter ConfigGetter, namespace, name string, external bool) (*clientcmdapi.Config, error) {
	release := releaseName(name)

	secret, err := getter.GetSecret(ctx, namespace, "vc-"+release)
	if err != nil {
		return nil, err
	}

	// Acquire the kubeconfig and hack it so that the server points to the
	// LoadBalancer endpoint.
	configBytes, ok := secret.Data["config"]
	if !ok {
		return nil, ErrConfigDataMissing
	}

	configStruct, err := clientcmd.NewClientConfigFromBytes(configBytes)
	if err != nil {
		return nil, err
	}

	config, err := configStruct.RawConfig()
	if err != nil {
		return nil, err
	}

	host := "https://" + release + "." + namespace + ":443"

	// You are responsible for setting up a port-forward in order to get access
	// to this instance, then the host API becomes the only attack surface to
	// worry about.
	if external {
		host = "https://localhost:8443"
	}

	config.Clusters["kubernetes"].Server = host

	return &config, nil
}

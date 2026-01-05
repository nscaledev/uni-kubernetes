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

package cluster

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/unikorn-cloud/core/pkg/constants"
	unikornv1 "github.com/unikorn-cloud/kubernetes/pkg/apis/unikorn/v1alpha1"
	"github.com/unikorn-cloud/kubernetes/pkg/monitor/upgrade/errors"
	"github.com/unikorn-cloud/kubernetes/pkg/monitor/upgrade/util"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Checker struct {
	client client.Client
}

func New(client client.Client) *Checker {
	return &Checker{
		client: client,
	}
}

func (c *Checker) upgradeResource(ctx context.Context, resource *unikornv1.KubernetesCluster, current, target *unikornv1.KubernetesClusterApplicationBundle) error {
	logger := log.FromContext(ctx)

	// If the current bundle is in preview, then don't offer to upgrade.
	if current.Spec.Preview {
		logger.Info("bundle in preview, ignoring")

		return nil
	}

	// If the current bundle is the best option already, we are done.
	if current.Name == target.Name {
		logger.Info("bundle already latest, ignoring")

		return nil
	}

	upgradable := util.UpgradeableResource(resource)

	if resource.Spec.ApplicationBundleAutoUpgrade == nil {
		if current.Spec.EndOfLife == nil || time.Now().Before(current.Spec.EndOfLife.Time) {
			logger.Info("resource auto-upgrade disabled, ignoring")

			return nil
		}

		logger.Info("resource auto-upgrade disabled, but bundle is end of life, forcing auto-upgrade")

		upgradable = util.NewForcedUpgradeResource(resource)
	}

	// Is it allowed to happen now?  Base it on the UID for ultimate randomness,
	// you can cause a stampede if all the resources are called "default".
	window := util.TimeWindowFromResource(ctx, upgradable)

	if !window.In() {
		logger.Info("not in upgrade window, ignoring", "start", window.Start, "end", window.End)

		return nil
	}

	logger.Info("bundle upgrading")

	resource.Spec.ApplicationBundle = target.Name

	if err := c.client.Update(ctx, resource); err != nil {
		return err
	}

	return nil
}

func (c *Checker) Check(ctx context.Context) error {
	logger := log.FromContext(ctx)

	logger.Info("checking for kubernetes cluster upgrades")

	allBundles := &unikornv1.KubernetesClusterApplicationBundleList{}

	if err := c.client.List(ctx, allBundles); err != nil {
		return err
	}

	resources := &unikornv1.KubernetesClusterList{}

	if err := c.client.List(ctx, resources); err != nil {
		return err
	}

	for i := range resources.Items {
		resource := &resources.Items[i]

		// What we need to do is respect semantic versioning, e.g. a major is a breaking
		// change therefore auto-upgrade is not allowed.
		currentBundle := allBundles.Get(resource.Spec.ApplicationBundle)
		if currentBundle == nil {
			return fmt.Errorf("%w: %s", errors.ErrMissingBundle, resource.Spec.ApplicationBundle)
		}

		discard := func(bundle unikornv1.KubernetesClusterApplicationBundle) bool {
			return bundle.Spec.Version.Major() != currentBundle.Spec.Version.Major()
		}

		allowedBundles := &unikornv1.KubernetesClusterApplicationBundleList{
			Items: slices.Clone(allBundles.Items),
		}

		allowedBundles.Items = slices.DeleteFunc(allowedBundles.Items, discard)

		// Extract the potential upgrade target bundles, these are sorted by version, so
		// the newest is on the top, we shall see why later...
		upgradeTagetBundles := allowedBundles.Upgradable()
		if len(upgradeTagetBundles.Items) == 0 {
			return errors.ErrNoBundles
		}

		slices.SortStableFunc(upgradeTagetBundles.Items, unikornv1.CompareKubernetesClusterApplicationBundle)

		// Pick the most recent as our upgrade target.
		upgradeTarget := &upgradeTagetBundles.Items[len(upgradeTagetBundles.Items)-1]

		logger := logger.WithValues(
			"organization", resource.Labels[constants.OrganizationLabel],
			"project", resource.Labels[constants.ProjectLabel],
			"cluster", resource.Name,
			"from", currentBundle.Spec.Version,
			"to", upgradeTarget.Spec.Version,
		)

		if err := c.upgradeResource(log.IntoContext(ctx, logger), resource, currentBundle, upgradeTarget); err != nil {
			return err
		}
	}

	return nil
}

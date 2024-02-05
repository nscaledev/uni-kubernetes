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

//nolint:revive,stylecheck
package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/unikorn-cloud/unikorn/pkg/server/errors"
	"github.com/unikorn-cloud/unikorn/pkg/server/generated"
	"github.com/unikorn-cloud/unikorn/pkg/server/handler/application"
	"github.com/unikorn-cloud/unikorn/pkg/server/handler/applicationbundle"
	"github.com/unikorn-cloud/unikorn/pkg/server/handler/cluster"
	"github.com/unikorn-cloud/unikorn/pkg/server/handler/controlplane"
	"github.com/unikorn-cloud/unikorn/pkg/server/handler/project"
	"github.com/unikorn-cloud/unikorn/pkg/server/handler/providers/openstack"
	"github.com/unikorn-cloud/unikorn/pkg/server/util"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	// client gives cached access to Kubernetes.
	client client.Client

	// options allows behaviour to be defined on the CLI.
	options *Options

	// openstack is the Openstack client.
	openstack *openstack.Openstack
}

func New(client client.Client, options *Options) (*Handler, error) {
	o, err := openstack.New(client, &options.Openstack)
	if err != nil {
		return nil, err
	}

	h := &Handler{
		client:    client,
		options:   options,
		openstack: o,
	}

	return h, nil
}

func (h *Handler) setCacheable(w http.ResponseWriter) {
	w.Header().Add("Cache-Control", fmt.Sprintf("max-age=%d", h.options.CacheMaxAge/time.Second))
	w.Header().Add("Cache-Control", "private")
}

func (h *Handler) setUncacheable(w http.ResponseWriter) {
	w.Header().Add("Cache-Control", "no-cache")
}

func (h *Handler) PostApiV1Project(w http.ResponseWriter, r *http.Request) {
	if err := project.NewClient(h.client).Create(r.Context()); err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) DeleteApiV1Project(w http.ResponseWriter, r *http.Request) {
	if err := project.NewClient(h.client).Delete(r.Context()); err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) GetApiV1Controlplanes(w http.ResponseWriter, r *http.Request) {
	result, err := controlplane.NewClient(h.client).List(r.Context())
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	util.WriteJSONResponse(w, r, http.StatusOK, result)
}

func (h *Handler) PostApiV1Controlplanes(w http.ResponseWriter, r *http.Request) {
	request := &generated.ControlPlane{}

	if err := util.ReadJSONBody(r, request); err != nil {
		errors.HandleError(w, r, err)
		return
	}

	if err := controlplane.NewClient(h.client).Create(r.Context(), request); err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) DeleteApiV1ControlplanesControlPlaneName(w http.ResponseWriter, r *http.Request, controlPlaneName generated.ControlPlaneNameParameter) {
	if err := controlplane.NewClient(h.client).Delete(r.Context(), controlPlaneName); err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) GetApiV1ControlplanesControlPlaneName(w http.ResponseWriter, r *http.Request, controlPlaneName generated.ControlPlaneNameParameter) {
	result, err := controlplane.NewClient(h.client).Get(r.Context(), controlPlaneName)
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	util.WriteJSONResponse(w, r, http.StatusOK, result)
}

func (h *Handler) PutApiV1ControlplanesControlPlaneName(w http.ResponseWriter, r *http.Request, controlPlaneName generated.ControlPlaneNameParameter) {
	request := &generated.ControlPlane{}

	if err := util.ReadJSONBody(r, request); err != nil {
		errors.HandleError(w, r, err)
		return
	}

	if err := controlplane.NewClient(h.client).Update(r.Context(), controlPlaneName, request); err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) GetApiV1ControlplanesControlPlaneNameClusters(w http.ResponseWriter, r *http.Request, controlPlaneName generated.ControlPlaneNameParameter) {
	result, err := cluster.NewClient(h.client, r, h.openstack).List(r.Context(), controlPlaneName)
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	util.WriteJSONResponse(w, r, http.StatusOK, result)
}

func (h *Handler) PostApiV1ControlplanesControlPlaneNameClusters(w http.ResponseWriter, r *http.Request, controlPlaneName generated.ControlPlaneNameParameter) {
	request := &generated.KubernetesCluster{}

	if err := util.ReadJSONBody(r, request); err != nil {
		errors.HandleError(w, r, err)
		return
	}

	if err := cluster.NewClient(h.client, r, h.openstack).Create(r.Context(), controlPlaneName, request); err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) DeleteApiV1ControlplanesControlPlaneNameClustersClusterName(w http.ResponseWriter, r *http.Request, controlPlaneName generated.ControlPlaneNameParameter, clusterName generated.ClusterNameParameter) {
	if err := cluster.NewClient(h.client, r, h.openstack).Delete(r.Context(), controlPlaneName, clusterName); err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) GetApiV1ControlplanesControlPlaneNameClustersClusterName(w http.ResponseWriter, r *http.Request, controlPlaneName generated.ControlPlaneNameParameter, clusterName generated.ClusterNameParameter) {
	result, err := cluster.NewClient(h.client, r, h.openstack).Get(r.Context(), controlPlaneName, clusterName)
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	util.WriteJSONResponse(w, r, http.StatusOK, result)
}

func (h *Handler) PutApiV1ControlplanesControlPlaneNameClustersClusterName(w http.ResponseWriter, r *http.Request, controlPlaneName generated.ControlPlaneNameParameter, clusterName generated.ClusterNameParameter) {
	request := &generated.KubernetesCluster{}

	if err := util.ReadJSONBody(r, request); err != nil {
		errors.HandleError(w, r, err)
		return
	}

	if err := cluster.NewClient(h.client, r, h.openstack).Update(r.Context(), controlPlaneName, clusterName, request); err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) GetApiV1ControlplanesControlPlaneNameClustersClusterNameKubeconfig(w http.ResponseWriter, r *http.Request, controlPlaneName generated.ControlPlaneNameParameter, clusterName generated.ClusterNameParameter) {
	result, err := cluster.NewClient(h.client, r, h.openstack).GetKubeconfig(r.Context(), controlPlaneName, clusterName)
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	util.WriteOctetStreamResponse(w, r, http.StatusOK, result)
}

func (h *Handler) GetApiV1ApplicationbundlesControlPlane(w http.ResponseWriter, r *http.Request) {
	result, err := applicationbundle.NewClient(h.client).ListControlPlane(r.Context())
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	util.WriteJSONResponse(w, r, http.StatusOK, result)
}

func (h *Handler) GetApiV1ApplicationbundlesCluster(w http.ResponseWriter, r *http.Request) {
	result, err := applicationbundle.NewClient(h.client).ListCluster(r.Context())
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	util.WriteJSONResponse(w, r, http.StatusOK, result)
}

func (h *Handler) GetApiV1Applications(w http.ResponseWriter, r *http.Request) {
	result, err := application.NewClient(h.client).List(r.Context())
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	util.WriteJSONResponse(w, r, http.StatusOK, result)
}

func (h *Handler) GetApiV1ProvidersOpenstackAvailabilityZonesCompute(w http.ResponseWriter, r *http.Request) {
	result, err := h.openstack.ListAvailabilityZonesCompute(r)
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setCacheable(w)
	util.WriteJSONResponse(w, r, http.StatusOK, result)
}

func (h *Handler) GetApiV1ProvidersOpenstackAvailabilityZonesBlockStorage(w http.ResponseWriter, r *http.Request) {
	result, err := h.openstack.ListAvailabilityZonesBlockStorage(r)
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setCacheable(w)
	util.WriteJSONResponse(w, r, http.StatusOK, result)
}

func (h *Handler) GetApiV1ProvidersOpenstackExternalNetworks(w http.ResponseWriter, r *http.Request) {
	result, err := h.openstack.ListExternalNetworks(r)
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setCacheable(w)
	util.WriteJSONResponse(w, r, http.StatusOK, result)
}

func (h *Handler) GetApiV1ProvidersOpenstackFlavors(w http.ResponseWriter, r *http.Request) {
	result, err := h.openstack.ListFlavors(r)
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setCacheable(w)
	util.WriteJSONResponse(w, r, http.StatusOK, result)
}

func (h *Handler) GetApiV1ProvidersOpenstackImages(w http.ResponseWriter, r *http.Request) {
	result, err := h.openstack.ListImages(r)
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setCacheable(w)
	util.WriteJSONResponse(w, r, http.StatusOK, result)
}

func (h *Handler) GetApiV1ProvidersOpenstackKeyPairs(w http.ResponseWriter, r *http.Request) {
	result, err := h.openstack.ListKeyPairs(r)
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	util.WriteJSONResponse(w, r, http.StatusOK, result)
}

func (h *Handler) GetApiV1ProvidersOpenstackProjects(w http.ResponseWriter, r *http.Request) {
	result, err := h.openstack.ListAvailableProjects(r)
	if err != nil {
		errors.HandleError(w, r, err)
		return
	}

	h.setUncacheable(w)
	util.WriteJSONResponse(w, r, http.StatusOK, result)
}

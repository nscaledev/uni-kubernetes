/*
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

//nolint:revive
package handler

import (
	"net/http"

	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
	"github.com/unikorn-cloud/core/pkg/server/util"
	"github.com/unikorn-cloud/kubernetes/pkg/constants"
)

func (h *Handler) GetApiV2Version(w http.ResponseWriter, r *http.Request) {
	result := coreapi.ServiceVersionRead{
		Name:    constants.Application,
		Version: constants.Version,
	}

	h.setUncacheable(w)
	util.WriteJSONResponse(w, r, http.StatusOK, result)
}

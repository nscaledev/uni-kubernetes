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

package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
	"github.com/unikorn-cloud/kubernetes/pkg/constants"
	"github.com/unikorn-cloud/kubernetes/pkg/server/handler"
)

func Test_Version_Get(t *testing.T) {
	t.Parallel()

	h := &handler.Handler{}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v2/version", nil)

	h.GetApiV2Version(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-cache", w.Header().Get("Cache-Control"))

	var result coreapi.ServiceVersionRead

	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Equal(t, constants.Application, result.Name)
	require.Equal(t, constants.Version, result.Version)
}

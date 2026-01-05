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

package handler

import (
	"net/http"

	"github.com/unikorn-cloud/core/pkg/server/errors"
)

// NotFound is called from the router when a path is not found.
func NotFound(w http.ResponseWriter, r *http.Request) {
	errors.HTTPNotFound().Write(w, r)
}

// MethodNotAllowed is called from the router when a method is not found for a path.
func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	errors.HTTPMethodNotAllowed().Write(w, r)
}

// HandleError is called when the router has trouble parsong paths.
func HandleError(w http.ResponseWriter, r *http.Request, err error) {
	errors.OAuth2InvalidRequest("invalid path/query element").WithError(err).Write(w, r)
}

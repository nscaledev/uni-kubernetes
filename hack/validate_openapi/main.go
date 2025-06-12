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

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/unikorn-cloud/kubernetes/pkg/openapi"
)

//nolint:gochecknoglobals
var failed bool

func report(v ...any) {
	fmt.Println(v...)

	failed = true
}

//nolint:gocognit,cyclop
func main() {
	spec, err := openapi.GetSwagger()
	if err != nil {
		report("failed to load spec", err)
	}

	if err := spec.Validate(context.Background()); err != nil {
		report("failed to validate spec", err)
	}

	for _, pathName := range spec.Paths.InMatchingOrder() {
		path := spec.Paths.Find(pathName)

		for method, operation := range path.Operations() {
			// Everything needs security defining.
			_, noSecurityAllowed := operation.Extensions["x-no-security-requirements"]

			if operation.Security == nil && !noSecurityAllowed {
				report("no security requirements set for ", method, pathName)
				os.Exit(1)
			}

			// If you have multiple, then the errors become ambiguous to handle.
			if operation.Security != nil && len(*operation.Security) != 1 {
				report("security requirement for", method, pathName, "require one security requirement")
				os.Exit(1)
			}

			//nolint:nestif
			if method == http.MethodGet {
				// Where there are responses, they must have a schema.
				for code := 100; code < 600; code++ {
					responseRef := operation.Responses.Status(code)
					if responseRef == nil {
						continue
					}

					if code != http.StatusOK {
						continue
					}

					response := responseRef.Value
					if response == nil {
						response = spec.Components.Responses[responseRef.Ref].Value
					}

					if response.Content == nil {
						report("no content type set for", code, method, pathName, "response")
					}

					for mimeType, mediaType := range response.Content {
						if mimeType == "application/json" && mediaType.Schema == nil {
							report("no schema set for", mimeType, code, method, pathName, "response")
						}
					}
				}
			}

			//nolint:nestif
			if method == http.MethodPost || method == http.MethodPut {
				// You have to explicitly opt out from following the rules.
				_, noBodyAllowed := operation.Extensions["x-no-body"]

				// POST/PUT calls will have something to validate.
				if operation.RequestBody == nil {
					if noBodyAllowed {
						continue
					}

					report("no request body set for", method, pathName)

					continue
				}

				body := operation.RequestBody.Value
				if body == nil {
					body = spec.Components.RequestBodies[operation.RequestBody.Ref].Value
				}

				// Request bodies will have a schema.
				for mimeType, mediaType := range body.Content {
					if mediaType.Schema == nil {
						report("no schema set for", mimeType, method, pathName)
					}
				}
			}
		}
	}

	if failed {
		os.Exit(1)
	}
}

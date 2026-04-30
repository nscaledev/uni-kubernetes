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

//nolint:testpackage
package server

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreclient "github.com/unikorn-cloud/core/pkg/client"
	identityclient "github.com/unikorn-cloud/identity/pkg/client"
	"github.com/unikorn-cloud/identity/pkg/middleware/authorization"
	openapimiddleware "github.com/unikorn-cloud/identity/pkg/middleware/openapi"
	identityopenapi "github.com/unikorn-cloud/identity/pkg/openapi"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	errTestUniAuthorizer = errors.New("uni authorizer error")
	errTestHTTPClient    = errors.New("http client error")
	errTestAuthorizer    = errors.New("passport authorizer error")
)

type stubAuthorizer struct{}

func (a *stubAuthorizer) Authorize(_ *openapi3filter.AuthenticationInput) (*authorization.Info, error) {
	return &authorization.Info{}, nil
}

func (a *stubAuthorizer) GetACL(_ context.Context, _ string) (*identityopenapi.Acl, error) {
	return &identityopenapi.Acl{}, nil
}

func TestServer_Authorizer(t *testing.T) {
	t.Parallel()

	t.Run("uses configured constructor", func(t *testing.T) {
		t.Parallel()

		server := &Server{
			IdentityOptions: identityclient.NewOptions(),
		}

		var (
			calledUniAuthorizer bool
			calledHTTPClient    bool
			calledAuthorizer    bool
			httpClient          = &http.Client{}
			uniAuthorizer       = &stubAuthorizer{}
			stub                = &stubAuthorizer{}
		)

		server.newUniAuthorizer = func(kubeClient client.Client, identityOptions *identityclient.Options, httpClientOptions *coreclient.HTTPClientOptions) (openapimiddleware.Authorizer, error) {
			calledUniAuthorizer = true

			assert.Nil(t, kubeClient)
			assert.Same(t, server.IdentityOptions, identityOptions)
			assert.Same(t, &server.ClientOptions, httpClientOptions)

			return uniAuthorizer, nil
		}

		server.newIdentityHTTPClient = func(kubeClient client.Client, identityOptions *identityclient.Options, httpClientOptions *coreclient.HTTPClientOptions) (*http.Client, error) {
			calledHTTPClient = true

			assert.Nil(t, kubeClient)
			assert.Same(t, server.IdentityOptions, identityOptions)
			assert.Same(t, &server.ClientOptions, httpClientOptions)

			return httpClient, nil
		}

		server.newAuthorizer = func(httpClientArg *http.Client, identityHost string, uniAuthorizerArg openapimiddleware.Authorizer) (openapimiddleware.Authorizer, error) {
			calledAuthorizer = true

			assert.Same(t, httpClient, httpClientArg)
			assert.Equal(t, server.IdentityOptions.Host(), identityHost)
			assert.Same(t, uniAuthorizer, uniAuthorizerArg)

			return stub, nil
		}

		authorizer, err := server.authorizer(nil)
		require.NoError(t, err)
		assert.True(t, calledUniAuthorizer)
		assert.True(t, calledHTTPClient)
		assert.True(t, calledAuthorizer)
		assert.Same(t, stub, authorizer)
	})

	t.Run("wraps uni authorizer errors", func(t *testing.T) {
		t.Parallel()

		s := &Server{
			IdentityOptions: identityclient.NewOptions(),
		}

		s.newUniAuthorizer = func(_ client.Client, _ *identityclient.Options, _ *coreclient.HTTPClientOptions) (openapimiddleware.Authorizer, error) {
			return nil, errTestUniAuthorizer
		}

		_, err := s.authorizer(nil)
		require.Error(t, err)
		require.ErrorIs(t, err, errTestUniAuthorizer)
		assert.Contains(t, err.Error(), "failed to initialize uni authorizer")
	})

	t.Run("wraps HTTP client errors", func(t *testing.T) {
		t.Parallel()

		s := &Server{
			IdentityOptions: identityclient.NewOptions(),
		}

		s.newUniAuthorizer = func(_ client.Client, _ *identityclient.Options, _ *coreclient.HTTPClientOptions) (openapimiddleware.Authorizer, error) {
			return &stubAuthorizer{}, nil
		}

		s.newIdentityHTTPClient = func(_ client.Client, _ *identityclient.Options, _ *coreclient.HTTPClientOptions) (*http.Client, error) {
			return nil, errTestHTTPClient
		}

		_, err := s.authorizer(nil)
		require.Error(t, err)
		require.ErrorIs(t, err, errTestHTTPClient)
		assert.Contains(t, err.Error(), "failed to initialize identity HTTP client")
	})

	t.Run("wraps passport authorizer errors", func(t *testing.T) {
		t.Parallel()

		s := &Server{
			IdentityOptions: identityclient.NewOptions(),
		}

		s.newUniAuthorizer = func(_ client.Client, _ *identityclient.Options, _ *coreclient.HTTPClientOptions) (openapimiddleware.Authorizer, error) {
			return &stubAuthorizer{}, nil
		}

		s.newIdentityHTTPClient = func(_ client.Client, _ *identityclient.Options, _ *coreclient.HTTPClientOptions) (*http.Client, error) {
			return &http.Client{}, nil
		}

		s.newAuthorizer = func(_ *http.Client, _ string, _ openapimiddleware.Authorizer) (openapimiddleware.Authorizer, error) {
			return nil, errTestAuthorizer
		}

		_, err := s.authorizer(nil)
		require.Error(t, err)
		require.ErrorIs(t, err, errTestAuthorizer)
		assert.Contains(t, err.Error(), "failed to initialize passport authorizer")
	})
}

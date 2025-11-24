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

package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"

	chi "github.com/go-chi/chi/v5"
	"github.com/spf13/pflag"
	"go.opentelemetry.io/otel/sdk/trace"

	coreclient "github.com/unikorn-cloud/core/pkg/client"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
	"github.com/unikorn-cloud/core/pkg/options"
	"github.com/unikorn-cloud/core/pkg/server/middleware/cors"
	"github.com/unikorn-cloud/core/pkg/server/middleware/opentelemetry"
	"github.com/unikorn-cloud/core/pkg/server/middleware/timeout"
	identityclient "github.com/unikorn-cloud/identity/pkg/client"
	"github.com/unikorn-cloud/identity/pkg/middleware/audit"
	openapimiddleware "github.com/unikorn-cloud/identity/pkg/middleware/openapi"
	openapimiddlewareremote "github.com/unikorn-cloud/identity/pkg/middleware/openapi/remote"
	"github.com/unikorn-cloud/kubernetes/pkg/constants"
	"github.com/unikorn-cloud/kubernetes/pkg/openapi"
	"github.com/unikorn-cloud/kubernetes/pkg/server/handler"
	regionclient "github.com/unikorn-cloud/region/pkg/client"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Server struct {
	// CoreOptions are all common across everything e.g. namespace.
	CoreOptions options.CoreOptions

	// ServerOptions are server specific options e.g. listener address etc.
	ServerOptions options.ServerOptions

	// HandlerOptions sets options for the HTTP handler.
	HandlerOptions handler.Options

	// CORSOptions are for remote resource sharing.
	CORSOptions cors.Options

	// ClientOptions are for generic TLS client options e.g. certificates.
	ClientOptions coreclient.HTTPClientOptions

	// IdentityOptions are for a shared identity client.
	IdentityOptions *identityclient.Options

	// RegionOptions are for a shared region client.
	RegionOptions *regionclient.Options

	// OpenAPIOptions are for OpenAPI processing.
	OpenAPIOptions openapimiddleware.Options
}

func (s *Server) AddFlags(flags *pflag.FlagSet) {
	if s.IdentityOptions == nil {
		s.IdentityOptions = identityclient.NewOptions()
	}

	if s.RegionOptions == nil {
		s.RegionOptions = regionclient.NewOptions()
	}

	s.CoreOptions.AddFlags(flags)
	s.ServerOptions.AddFlags(flags)
	s.HandlerOptions.AddFlags(flags)
	s.CORSOptions.AddFlags(flags)
	s.ClientOptions.AddFlags(flags)
	s.IdentityOptions.AddFlags(flags)
	s.RegionOptions.AddFlags(flags)
	s.OpenAPIOptions.AddFlags(flags)
}

func (s *Server) SetupLogging() {
	s.CoreOptions.SetupLogging()
}

// SetupOpenTelemetry adds a span processor that will print root spans to the
// logs by default, and optionally ship the spans to an OTLP listener.
// TODO: move config into an otel specific options struct.
func (s *Server) SetupOpenTelemetry(ctx context.Context) error {
	return s.CoreOptions.SetupOpenTelemetry(ctx, trace.WithSpanProcessor(&opentelemetry.LoggingSpanProcessor{}))
}

func (s *Server) GetServer(client client.Client) (*http.Server, error) {
	pprofHandler := http.NewServeMux()
	pprofHandler.HandleFunc("/debug/pprof/", pprof.Index)
	pprofHandler.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	pprofHandler.HandleFunc("/debug/pprof/profile", pprof.Profile)
	pprofHandler.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	pprofHandler.HandleFunc("/debug/pprof/trace", pprof.Trace)

	go func() {
		pprofServer := http.Server{
			Addr:              ":6060",
			ReadTimeout:       s.ServerOptions.ReadTimeout,
			ReadHeaderTimeout: s.ServerOptions.ReadHeaderTimeout,
			WriteTimeout:      s.ServerOptions.WriteTimeout,
			Handler:           pprofHandler,
		}

		if err := pprofServer.ListenAndServe(); err != nil {
			fmt.Println(err)
		}
	}()

	schema, err := coreapi.NewSchema(openapi.GetSwagger)
	if err != nil {
		return nil, err
	}

	// Middleware specified here is applied to all requests pre-routing.
	router := chi.NewRouter()
	router.Use(timeout.Middleware(s.ServerOptions.RequestTimeout))
	router.Use(opentelemetry.Middleware(constants.Application, constants.Version))
	router.Use(cors.Middleware(schema, &s.CORSOptions))
	router.NotFound(http.HandlerFunc(handler.NotFound))
	router.MethodNotAllowed(http.HandlerFunc(handler.MethodNotAllowed))

	authorizer, err := openapimiddlewareremote.NewAuthorizer(client, s.IdentityOptions, &s.ClientOptions)
	if err != nil {
		return nil, err
	}

	// Middleware specified here is applied to all requests post-routing.
	// NOTE: these are applied in reverse order!!
	chiServerOptions := openapi.ChiServerOptions{
		BaseRouter:       router,
		ErrorHandlerFunc: handler.HandleError,
		Middlewares: []openapi.MiddlewareFunc{
			audit.Middleware(schema, constants.Application, constants.Version),
			openapimiddleware.Middleware(&s.OpenAPIOptions, authorizer, schema),
		},
	}

	// NOTE: any clients that are used, must issue new tokens as this service to
	// prevent the user having to be granted excessive privilege.
	issuer := identityclient.NewTokenIssuer(client, s.IdentityOptions, &s.ClientOptions, constants.ServiceDescriptor())

	identity, err := identityclient.New(client, s.IdentityOptions, &s.ClientOptions).APIClient(context.TODO(), issuer)
	if err != nil {
		return nil, err
	}

	region, err := regionclient.New(client, s.RegionOptions, &s.ClientOptions).APIClient(context.TODO(), issuer)
	if err != nil {
		return nil, err
	}

	handlerInterface, err := handler.New(client, s.CoreOptions.Namespace, &s.HandlerOptions, identity, region)
	if err != nil {
		return nil, err
	}

	server := &http.Server{
		Addr:              s.ServerOptions.ListenAddress,
		ReadTimeout:       s.ServerOptions.ReadTimeout,
		ReadHeaderTimeout: s.ServerOptions.ReadHeaderTimeout,
		WriteTimeout:      s.ServerOptions.WriteTimeout,
		Handler:           openapi.HandlerWithOptions(handlerInterface, chiServerOptions),
	}

	return server, nil
}

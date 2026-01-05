/*
Copyright 2025 the Unikorn Authors.
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

package openapi

import (
	"context"
	"net/http"
)

type Builder struct {
	options []ClientOption
}

func NewBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) WithHTTPClient(client *http.Client) {
	b.options = append(b.options, WithHTTPClient(client))
}

func (b *Builder) WithRequestEditorFn(fn func(context.Context, *http.Request) error) {
	b.options = append(b.options, WithRequestEditorFn(fn))
}

func (b *Builder) Client(hostname string) (*ClientWithResponses, error) {
	return NewClientWithResponses(hostname, b.options...)
}

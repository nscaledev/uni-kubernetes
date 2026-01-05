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

package constants

import (
	"fmt"
	"os"
	"path"

	"github.com/unikorn-cloud/core/pkg/util"
)

var (
	// Application is the application name.
	//nolint:gochecknoglobals
	Application = path.Base(os.Args[0])

	// Version is the application version set via the Makefile.
	//nolint:gochecknoglobals
	Version string

	// Revision is the git revision set via the Makefile.
	//nolint:gochecknoglobals
	Revision string
)

// VersionString returns a canonical version string.  It's based on
// HTTP's User-Agent so can be used to set that too, if this ever has to
// call out ot other micro services.
func VersionString() string {
	return fmt.Sprintf("%s/%s (revision/%s)", Application, Version, Revision)
}

func ServiceDescriptor() util.ServiceDescriptor {
	return util.ServiceDescriptor{
		Name:     Application,
		Version:  Version,
		Revision: Revision,
	}
}

// Copyright 2019-2026 The Liqo Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package openvpn

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/liqotech/liqo/pkg/liqoctl/rest"
)

// Options encapsulates the arguments of the openvpn command.
type Options struct {
	createOptions   *rest.CreateOptions
	generateOptions *rest.GenerateOptions

	ServerSecretName string
	ClientSecretName string
}

var _ rest.API = &Options{}

// OpenVPN returns the rest API for the openvpn command.
func OpenVPN() rest.API {
	return &Options{}
}

// APIOptions returns the APIOptions for the openvpn API.
func (o *Options) APIOptions() *rest.APIOptions {
	return &rest.APIOptions{
		EnableGenerate: true,
	}
}

// Create implements the Create method for the API interface (not implemented for openvpn).
func (o *Options) Create(ctx context.Context, options *rest.CreateOptions) *cobra.Command {
	return nil
}

// Delete implements the Delete method for the API interface (not implemented for openvpn).
func (o *Options) Delete(ctx context.Context, options *rest.DeleteOptions) *cobra.Command {
	return nil
}

// Get implements the Get method for the API interface (not implemented for openvpn).
func (o *Options) Get(ctx context.Context, options *rest.GetOptions) *cobra.Command {
	return nil
}

// Update implements the Update method for the API interface (not implemented for openvpn).
func (o *Options) Update(ctx context.Context, options *rest.UpdateOptions) *cobra.Command {
	return nil
}

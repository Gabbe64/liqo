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
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/cli-runtime/pkg/printers"

	"github.com/liqotech/liqo/pkg/liqoctl/completion"
	"github.com/liqotech/liqo/pkg/liqoctl/output"
	"github.com/liqotech/liqo/pkg/liqoctl/rest"
	"github.com/liqotech/liqo/pkg/utils/args"
)

const liqoctlGenerateOpenVPNHelp = `Generate OpenVPN certificates and keys for gateway servers and clients.

This command generates all necessary certificates and keys for OpenVPN gateways:
- CA certificate and key
- Server certificate and key
- Client certificate and key
- Diffie-Hellman parameters
- TLS authentication static key

The output consists of two Kubernetes Secrets:
1. Server Secret: Contains server certificate, key, CA, DH parameters, and TLS auth key
2. Client Secret: Contains client certificate, key, CA, and TLS auth key

These secrets can be applied to your cluster using kubectl apply.

Examples:
  $ {{ .Executable }} generate openvpn
  $ {{ .Executable }} generate openvpn --server-secret-name my-server --client-secret-name my-client
  $ {{ .Executable }} generate openvpn -o yaml | kubectl apply -f -`

// Generate generates OpenVPN certificates and secrets.
func (o *Options) Generate(ctx context.Context, options *rest.GenerateOptions) *cobra.Command {
	outputFormat := args.NewEnum([]string{"json", "yaml"}, "yaml")

	o.generateOptions = options

	cmd := &cobra.Command{
		Use:   "openvpn",
		Short: "Generate OpenVPN certificates and keys",
		Long:  liqoctlGenerateOpenVPNHelp,
		Args:  cobra.NoArgs,

		PreRun: func(_ *cobra.Command, _ []string) {
			options.OutputFormat = outputFormat.Value
			o.generateOptions = options
		},

		Run: func(_ *cobra.Command, _ []string) {
			output.ExitOnErr(o.handleGenerate())
		},
	}

	cmd.Flags().VarP(outputFormat, "output", "o",
		"Output format of the resulting Secret resources. Supported formats: json, yaml")

	cmd.Flags().StringVar(&o.ServerSecretName, "server-secret-name", "openvpn-server",
		"The name of the Kubernetes Secret for the server certificates")
	cmd.Flags().StringVar(&o.ClientSecretName, "client-secret-name", "openvpn-client",
		"The name of the Kubernetes Secret for the client certificates")

	runtime.Must(cmd.RegisterFlagCompletionFunc("output", completion.Enumeration(outputFormat.Allowed)))

	return cmd
}

func (o *Options) handleGenerate() error {
	opts := o.generateOptions

	// Generate all certificates and keys
	bundle, err := GenerateCertificateBundle()
	if err != nil {
		opts.Printer.CheckErr(fmt.Errorf("failed to generate certificate bundle: %w", err))
		return err
	}

	// Forge the server and client secrets
	serverSecret := ForgeServerSecret(opts.Namespace, o.ServerSecretName, bundle)
	clientSecret := ForgeClientSecret(opts.Namespace, o.ClientSecretName, bundle)

	// Determine output format
	var printer printers.ResourcePrinter
	switch opts.OutputFormat {
	case "yaml":
		printer = &printers.YAMLPrinter{}
	case "json":
		printer = &printers.JSONPrinter{}
	default:
		return fmt.Errorf("unsupported output format %q", opts.OutputFormat)
	}

	// Print server secret
	if err := printer.PrintObj(serverSecret, os.Stdout); err != nil {
		opts.Printer.CheckErr(fmt.Errorf("failed to print server secret: %w", err))
		return err
	}

	// Print separator for YAML (---) or newline for JSON
	if opts.OutputFormat == "yaml" {
		fmt.Println("---")
	}

	// Print client secret
	if err := printer.PrintObj(clientSecret, os.Stdout); err != nil {
		opts.Printer.CheckErr(fmt.Errorf("failed to print client secret: %w", err))
		return err
	}

	return nil
}

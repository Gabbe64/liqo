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

// Package fou contains the logic to configure the FOU tunnel runtime.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	networkingv1beta1 "github.com/liqotech/liqo/apis/networking/v1beta1"
	"github.com/liqotech/liqo/pkg/gateway"
	"github.com/liqotech/liqo/pkg/gateway/concurrent"
	"github.com/liqotech/liqo/pkg/gateway/tunnel/fou"
	flagsutils "github.com/liqotech/liqo/pkg/utils/flags"
	"github.com/liqotech/liqo/pkg/utils/mapper"
	"github.com/liqotech/liqo/pkg/utils/restcfg"
)

var (
	scheme  = runtime.NewScheme()
	options = fou.NewOptions(gateway.NewOptions())
)

func init() {
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(networkingv1beta1.AddToScheme(scheme))
}

// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;create;update;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete

func main() {
	var cmd = cobra.Command{
		Use:  "liqo-fou",
		RunE: run,
	}

	flagsutils.InitKlogFlags(cmd.Flags())
	restcfg.InitFlags(cmd.Flags())

	gateway.InitFlags(cmd.Flags(), options.GwOptions)
	fou.InitFlags(cmd.Flags(), options)
	if err := fou.MarkFlagsRequired(&cmd, options); err != nil {
		klog.Error(err)
		os.Exit(1)
	}

	if err := cmd.Execute(); err != nil {
		klog.Error(err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, _ []string) error {
	// Set controller-runtime logger.
	log.SetLogger(klog.NewKlogr())

	// Setup FOU tunnel interface.
	if err := fou.InitFouLink(cmd.Context(), options); err != nil {
		return fmt.Errorf("unable to init FOU link: %w", err)
	}
	// Deregister the FOU receive port when the runtime shuts down.
	defer fou.CleanupFouPort(options.LocalPort)

	// Get the rest config.
	cfg := config.GetConfigOrDie()

	// Create the manager.
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		MapperProvider: mapper.LiqoMapperProvider(scheme),
		Scheme:         scheme,
		Metrics: server.Options{
			BindAddress: options.GwOptions.MetricsAddress,
		},
		HealthProbeBindAddress: options.GwOptions.ProbeAddr,
		LeaderElection:         false,
	})
	if err != nil {
		return fmt.Errorf("unable to create manager: %w", err)
	}

	// Register the healthiness probes.
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up healthz probe: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up readyz probe: %w", err)
	}

	if options.GwOptions.LeaderElection {
		runnable, err := concurrent.NewRunnableGuest(options.GwOptions.ContainerName)
		if err != nil {
			return fmt.Errorf("unable to create runnable guest: %w", err)
		}
		if err := runnable.Start(cmd.Context()); err != nil {
			return fmt.Errorf("unable to start runnable guest: %w", err)
		}
		defer runnable.Close()
	}

	// Start the manager.
	return mgr.Start(cmd.Context())
}

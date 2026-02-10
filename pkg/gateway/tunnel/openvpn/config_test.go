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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liqotech/liqo/pkg/gateway"
)

func TestBuildConfig(t *testing.T) {
	opts := NewOptions(gateway.NewOptions())
	opts.GwOptions.Mode = gateway.ModeClient
	opts.EndpointAddress = "10.0.0.1"
	opts.IfconfigLocalIP = "169.254.18.2"
	opts.IfconfigRemoteIP = "169.254.18.1"
	opts.TLSClient = true

	cfg := BuildConfig(opts)

	checks := []string{
		"dev liqo-tunnel\n",
		"dev-type tun\n",
		"ifconfig 169.254.18.2 169.254.18.1\n",
		"ca /etc/openvpn/keys/ca.crt\n",
		"cert /etc/openvpn/keys/client.crt\n",
		"key /etc/openvpn/keys/client.key\n",
		"tls-auth /etc/openvpn/keys/ta.key 1\n",
		"tls-client\n",
		"cipher AES-256-CBC\n",
		"auth SHA256\n",
		"proto udp\n",
		"keepalive 10 120\n",
		"mtu 1340\n",
		"remote 10.0.0.1\n",
		"port 51840\n",
	}

	for _, needle := range checks {
		if !strings.Contains(cfg, needle) {
			t.Fatalf("expected config to contain %q, got: %s", needle, cfg)
		}
	}
}

func TestWriteConfigFile(t *testing.T) {
	opts := NewOptions(gateway.NewOptions())
	opts.ConfigDir = t.TempDir()
	opts.GwOptions.Mode = gateway.ModeClient
	opts.EndpointAddress = "10.0.0.1"
	opts.IfconfigLocalIP = "169.254.18.2"
	opts.IfconfigRemoteIP = "169.254.18.1"

	path, err := WriteConfigFile(opts, "")
	if err != nil {
		t.Fatalf("WriteConfigFile failed: %v", err)
	}

	if filepath.Base(path) != "openvpn.conf" {
		t.Fatalf("unexpected file name: %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("unable to read config file: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("config file is empty")
	}
}

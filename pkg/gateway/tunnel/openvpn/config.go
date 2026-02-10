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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/liqotech/liqo/pkg/gateway"
)

// BuildConfig renders the OpenVPN configuration file from the provided options.
func BuildConfig(opts *Options) string {
	var b strings.Builder

	// Writer functions for different types of options.
	writeKV := func(key string, value string) {
		if value == "" {
			return
		}
		b.WriteString(key)
		b.WriteByte(' ')
		b.WriteString(value)
		b.WriteByte('\n')
	}
	writeBool := func(key string, enabled bool) {
		if !enabled {
			return
		}
		b.WriteString(key)
		b.WriteByte('\n')
	}
	writeInt := func(key string, value int) {
		if value == 0 {
			return
		}
		b.WriteString(key)
		b.WriteByte(' ')
		b.WriteString(fmt.Sprintf("%d", value))
		b.WriteByte('\n')
	}

	writeKV("dev", opts.Device)
	writeKV("dev-type", opts.DeviceType)
	if opts.IfconfigLocalIP != "" && opts.IfconfigRemoteIP != "" {
		b.WriteString("ifconfig ")
		b.WriteString(opts.IfconfigLocalIP)
		b.WriteByte(' ')
		b.WriteString(opts.IfconfigRemoteIP)
		b.WriteByte('\n')
	}
	writeKV("ca", opts.CAFile)
	writeKV("cert", opts.CertFile)
	writeKV("key", opts.KeyFile)
	writeKV("dh", opts.DHFile)
	// tls-auth requires both the key file and the direction.
	// example: tls-auth /etc/openvpn/keys/ta.key 1
	if opts.TLSAuthKeyFile != "" {
		b.WriteString("tls-auth ")
		b.WriteString(opts.TLSAuthKeyFile)
		b.WriteByte(' ')
		b.WriteString(fmt.Sprintf("%d", opts.TLSAuthDirection))
		b.WriteByte('\n')
	}
	writeBool("tls-client", opts.TLSClient)
	writeBool("tls-server", opts.TLSServer)
	writeKV("cipher", opts.Cipher)
	writeKV("auth", opts.Auth)
	writeKV("proto", opts.Proto)
	if opts.KeepalivePing > 0 && opts.KeepaliveTimeout > 0 {
		b.WriteString("keepalive ")
		b.WriteString(fmt.Sprintf("%d %d", opts.KeepalivePing, opts.KeepaliveTimeout))
		b.WriteByte('\n')
	}
	// TODO: handle the mtu case.
	// writeInt("mtu", opts.MTU)
	writeInt("max-clients", opts.MaxClients)

	if opts.GwOptions != nil && opts.GwOptions.Mode == gateway.ModeClient {
		writeKV("remote", opts.EndpointAddress)
		writeInt("port", opts.EndpointPort)
		return b.String()
	}
	writeInt("port", opts.ListenPort)

	return b.String()
}

// WriteConfigFile writes the OpenVPN configuration to a file and returns its path.
// If filePath is empty, it defaults to <ConfigDir>/openvpn.conf.
func WriteConfigFile(opts *Options, filePath string) (string, error) {
	if filePath == "" {
		filePath = filepath.Join(opts.ConfigDir, "openvpn.conf")
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return "", fmt.Errorf("unable to create config directory: %w", err)
	}

	content := BuildConfig(opts)
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("unable to write config file: %w", err)
	}

	return filePath, nil
}

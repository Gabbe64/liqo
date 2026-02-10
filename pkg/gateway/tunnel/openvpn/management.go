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
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// ManagementClient provides minimal access to the OpenVPN management interface.
type ManagementClient struct {
	Address string
	Timeout time.Duration
	Dialer  *net.Dialer
}

// ManagementState represents the OpenVPN state line.
type ManagementState struct {
	State  string
	Detail string
	Raw    string
}

// TunnelStats represents TUN/TAP byte counters from the management status output.
type TunnelStats struct {
	ReadBytes  uint64
	WriteBytes uint64
}

// NewManagementClient returns a new ManagementClient for the given address.
func NewManagementClient(address string, timeout time.Duration) *ManagementClient {
	return &ManagementClient{
		Address: address,
		Timeout: timeout,
		Dialer:  &net.Dialer{Timeout: timeout},
	}
}

// GetState queries the management interface and parses the STATE line.
func (mc *ManagementClient) GetState() (*ManagementState, error) {
	lines, err := mc.command("state")
	if err != nil {
		return nil, err
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "STATE,") {
			parts := strings.Split(line, ",")
			state := &ManagementState{Raw: line}
			if len(parts) >= 3 {
				state.State = parts[2]
			}
			if len(parts) >= 4 {
				state.Detail = parts[3]
			}
			return state, nil
		}
	}
	return nil, fmt.Errorf("state line not found in management output")
}

// IsInterfaceUp returns true when OpenVPN reports a CONNECTED state.
func (mc *ManagementClient) IsInterfaceUp() (bool, error) {
	state, err := mc.GetState()
	if err != nil {
		return false, err
	}
	return strings.EqualFold(state.State, "CONNECTED"), nil
}

// GetTunnelStats queries the management interface and parses TUN/TAP byte counters.
func (mc *ManagementClient) GetTunnelStats() (*TunnelStats, error) {
	lines, err := mc.command("status 2")
	if err != nil {
		return nil, err
	}
	stats := &TunnelStats{}
	for _, line := range lines {
		if strings.HasPrefix(line, "TUN/TAP read bytes,") {
			value := strings.TrimPrefix(line, "TUN/TAP read bytes,")
			if v, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64); err == nil {
				stats.ReadBytes = v
			}
		}
		if strings.HasPrefix(line, "TUN/TAP write bytes,") {
			value := strings.TrimPrefix(line, "TUN/TAP write bytes,")
			if v, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64); err == nil {
				stats.WriteBytes = v
			}
		}
	}
	return stats, nil
}

func (mc *ManagementClient) command(cmd string) ([]string, error) {
	conn, err := mc.Dialer.Dial("tcp", mc.Address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if mc.Timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(mc.Timeout))
	}

	if _, err := fmt.Fprintf(conn, "%s\n", cmd); err != nil {
		return nil, err
	}
	if _, err := fmt.Fprint(conn, "quit\n"); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(conn)
	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

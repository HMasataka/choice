package webrtc

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func TestDefaultICEConfig(t *testing.T) {
	config := DefaultICEConfig()

	// Verify ICE Lite is enabled by default
	if !config.Lite {
		t.Error("ICE Lite should be enabled by default")
	}

	// Verify port range
	if config.UDPPortRange.Min != DefaultUDPPortMin {
		t.Errorf("expected min port %d, got %d", DefaultUDPPortMin, config.UDPPortRange.Min)
	}
	if config.UDPPortRange.Max != DefaultUDPPortMax {
		t.Errorf("expected max port %d, got %d", DefaultUDPPortMax, config.UDPPortRange.Max)
	}

	// Verify timeout
	if config.ConnectionTimeout != DefaultConnectionTimeout {
		t.Errorf("expected connection timeout %v, got %v", DefaultConnectionTimeout, config.ConnectionTimeout)
	}

	// Verify IPv4 priority
	if !config.PreferIPv4 {
		t.Error("IPv4 should be preferred by default")
	}

	// Verify dual-stack is enabled
	if config.DisableIPv6 {
		t.Error("IPv6 should not be disabled by default")
	}

	// Verify network types include both UDP4 and UDP6
	hasUDP4 := false
	hasUDP6 := false
	for _, nt := range config.NetworkTypes {
		if nt == NetworkTypeUDP4 {
			hasUDP4 = true
		}
		if nt == NetworkTypeUDP6 {
			hasUDP6 = true
		}
	}
	if !hasUDP4 {
		t.Error("NetworkTypeUDP4 should be in default network types")
	}
	if !hasUDP6 {
		t.Error("NetworkTypeUDP6 should be in default network types")
	}

	// Verify candidate types (only host for ICE Lite)
	if len(config.CandidateTypes) != 1 || config.CandidateTypes[0] != CandidateTypeHost {
		t.Error("ICE Lite should only use host candidates")
	}
}

func TestICELiteConfig(t *testing.T) {
	publicIPs := []string{"203.0.113.1", "198.51.100.1"}
	config := ICELiteConfig(publicIPs)

	if !config.Lite {
		t.Error("ICE Lite mode should be enabled")
	}

	if len(config.NAT1To1IPs) != 2 {
		t.Errorf("expected 2 NAT 1:1 IPs, got %d", len(config.NAT1To1IPs))
	}
}

func TestValidateICEConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  ICEConfig
		wantErr error
	}{
		{
			name: "valid ICE Lite config with public IPs",
			config: ICEConfig{
				Lite:       true,
				NAT1To1IPs: []string{"203.0.113.1"},
				UDPPortRange: PortRange{
					Min: 10000,
					Max: 20000,
				},
			},
			wantErr: nil,
		},
		{
			name: "valid non-Lite config without public IPs",
			config: ICEConfig{
				Lite: false,
				UDPPortRange: PortRange{
					Min: 10000,
					Max: 20000,
				},
			},
			wantErr: nil,
		},
		{
			name: "invalid port range - min >= max",
			config: ICEConfig{
				Lite:       true,
				NAT1To1IPs: []string{"203.0.113.1"},
				UDPPortRange: PortRange{
					Min: 20000,
					Max: 10000,
				},
			},
			wantErr: ErrInvalidPortRange,
		},
		{
			name: "invalid port range - too small (51 ports)",
			config: ICEConfig{
				Lite:       true,
				NAT1To1IPs: []string{"203.0.113.1"},
				UDPPortRange: PortRange{
					Min: 10000,
					Max: 10050,
				},
			},
			wantErr: ErrPortRangeTooSmall,
		},
		{
			name: "invalid port range - too small (99 ports)",
			config: ICEConfig{
				Lite:       true,
				NAT1To1IPs: []string{"203.0.113.1"},
				UDPPortRange: PortRange{
					Min: 10000,
					Max: 10098, // 99 ports (10098 - 10000 + 1)
				},
			},
			wantErr: ErrPortRangeTooSmall,
		},
		{
			name: "valid port range - exactly 100 ports",
			config: ICEConfig{
				Lite:       true,
				NAT1To1IPs: []string{"203.0.113.1"},
				UDPPortRange: PortRange{
					Min: 10000,
					Max: 10099, // 100 ports (10099 - 10000 + 1)
				},
			},
			wantErr: nil,
		},
		{
			name: "invalid IP address",
			config: ICEConfig{
				Lite:       true,
				NAT1To1IPs: []string{"not-an-ip"},
				UDPPortRange: PortRange{
					Min: 10000,
					Max: 20000,
				},
			},
			wantErr: ErrInvalidIP,
		},
		{
			name: "ICE Lite without public IP",
			config: ICEConfig{
				Lite:       true,
				NAT1To1IPs: nil,
				UDPPortRange: PortRange{
					Min: 10000,
					Max: 20000,
				},
			},
			wantErr: ErrNoPublicIP,
		},
		{
			name: "valid IPv6 addresses",
			config: ICEConfig{
				Lite:       true,
				NAT1To1IPs: []string{"2001:db8::1"},
				UDPPortRange: PortRange{
					Min: 10000,
					Max: 20000,
				},
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateICEConfig(tt.config)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateICEConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestApplyToSettingEngine(t *testing.T) {
	tests := []struct {
		name    string
		config  ICEConfig
		wantErr bool
	}{
		{
			name: "default config with public IP",
			config: ICEConfig{
				Lite:       true,
				NAT1To1IPs: []string{"203.0.113.1"},
				UDPPortRange: PortRange{
					Min: 10000,
					Max: 20000,
				},
				NetworkTypes:        []NetworkType{NetworkTypeUDP4, NetworkTypeUDP6},
				DisconnectedTimeout: DefaultDisconnectedTimeout,
				FailedTimeout:       DefaultFailedTimeout,
				KeepaliveInterval:   DefaultKeepaliveInterval,
			},
			wantErr: false,
		},
		{
			name: "config with IPv6 disabled",
			config: ICEConfig{
				Lite:       true,
				NAT1To1IPs: []string{"203.0.113.1"},
				UDPPortRange: PortRange{
					Min: 10000,
					Max: 20000,
				},
				NetworkTypes:        []NetworkType{NetworkTypeUDP4, NetworkTypeUDP6},
				DisableIPv6:         true,
				DisconnectedTimeout: DefaultDisconnectedTimeout,
				FailedTimeout:       DefaultFailedTimeout,
				KeepaliveInterval:   DefaultKeepaliveInterval,
			},
			wantErr: false,
		},
		{
			name: "config with interface filter",
			config: ICEConfig{
				Lite:       true,
				NAT1To1IPs: []string{"203.0.113.1"},
				UDPPortRange: PortRange{
					Min: 10000,
					Max: 20000,
				},
				InterfaceFilter: func(name string) bool {
					return name == "eth0"
				},
				DisconnectedTimeout: DefaultDisconnectedTimeout,
				FailedTimeout:       DefaultFailedTimeout,
				KeepaliveInterval:   DefaultKeepaliveInterval,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			se := webrtc.SettingEngine{}
			err := ApplyToSettingEngine(&se, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ApplyToSettingEngine() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestICEServerConfig(t *testing.T) {
	config := ICEServerConfig{
		URLs:       []string{"stun:stun.example.com:3478"},
		Username:   "user",
		Credential: "pass",
	}

	server := config.ToWebRTCICEServer()

	if len(server.URLs) != 1 || server.URLs[0] != "stun:stun.example.com:3478" {
		t.Error("URLs not converted correctly")
	}
	if server.Username != "user" {
		t.Error("Username not converted correctly")
	}
	if server.Credential != "pass" {
		t.Error("Credential not converted correctly")
	}
}

func TestICEServersConfig(t *testing.T) {
	config := ICEServersConfig{
		Servers: []ICEServerConfig{
			{URLs: []string{"stun:stun1.example.com:3478"}},
			{URLs: []string{"stun:stun2.example.com:3478"}},
		},
	}

	servers := config.ToWebRTCICEServers()

	if len(servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(servers))
	}
}

func TestDefaultSTUNServers(t *testing.T) {
	servers := DefaultSTUNServers()

	if len(servers.Servers) == 0 {
		t.Error("default STUN servers should not be empty")
	}

	// Verify all servers have URLs
	for i, s := range servers.Servers {
		if len(s.URLs) == 0 {
			t.Errorf("server %d has no URLs", i)
		}
	}
}

func TestCandidatePriority(t *testing.T) {
	// Test priority calculation per RFC 5245
	// Priority = (2^24 * type_preference) + (2^8 * local_preference) + (256 - component_id)

	priority := CandidatePriority{
		TypePreference:  126, // Host
		LocalPreference: 65535,
		ComponentID:     1, // RTP
	}

	calculated := priority.Calculate()
	expected := uint32(126<<24) + uint32(65535<<8) + (256 - 1)

	if calculated != expected {
		t.Errorf("expected priority %d, got %d", expected, calculated)
	}
}

func TestDefaultCandidatePriorities(t *testing.T) {
	priorities := DefaultCandidatePriorities()

	// Host should have highest priority
	if priorities[CandidateTypeHost] <= priorities[CandidateTypeSrflx] {
		t.Error("host priority should be higher than srflx")
	}
	if priorities[CandidateTypeSrflx] <= priorities[CandidateTypeRelay] {
		t.Error("srflx priority should be higher than relay")
	}
}

func TestIPv4IPv6Priority(t *testing.T) {
	ipv4Prio := IPv4Priority()
	ipv6Prio := IPv6Priority()

	if ipv4Prio <= ipv6Prio {
		t.Error("IPv4 priority should be higher than IPv6")
	}
}

func TestCreateInterfaceFilter(t *testing.T) {
	// Empty list returns nil
	filter := CreateInterfaceFilter(nil)
	if filter != nil {
		t.Error("empty list should return nil filter")
	}

	// Non-empty list returns filter function
	filter = CreateInterfaceFilter([]string{"eth0", "eth1"})
	if filter == nil {
		t.Error("non-empty list should return filter function")
	}

	// Filter should allow listed interfaces
	if !filter("eth0") {
		t.Error("filter should allow eth0")
	}
	if !filter("eth1") {
		t.Error("filter should allow eth1")
	}

	// Filter should reject unlisted interfaces
	if filter("eth2") {
		t.Error("filter should reject eth2")
	}
}

func TestCreateIPFilter(t *testing.T) {
	// Empty list returns nil
	filter, err := CreateIPFilter(nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if filter != nil {
		t.Error("empty list should return nil filter")
	}

	// Valid CIDRs
	filter, err = CreateIPFilter([]string{"192.168.0.0/24", "10.0.0.0/8"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if filter == nil {
		t.Error("valid CIDRs should return filter function")
	}

	// Test filter
	if !filter(net.ParseIP("192.168.0.1")) {
		t.Error("filter should allow 192.168.0.1")
	}
	if !filter(net.ParseIP("10.1.2.3")) {
		t.Error("filter should allow 10.1.2.3")
	}
	if filter(net.ParseIP("8.8.8.8")) {
		t.Error("filter should reject 8.8.8.8")
	}

	// Invalid CIDR
	_, err = CreateIPFilter([]string{"invalid-cidr"})
	if err == nil {
		t.Error("invalid CIDR should return error")
	}
}

func TestICEConfigCopy(t *testing.T) {
	original := ICEConfig{
		Lite:                true,
		NAT1To1IPs:          []string{"203.0.113.1", "203.0.113.2"},
		UDPPortRange:        PortRange{Min: 10000, Max: 20000},
		NetworkTypes:        []NetworkType{NetworkTypeUDP4, NetworkTypeUDP6},
		DisableIPv6:         false,
		PreferIPv4:          true,
		ConnectionTimeout:   30 * time.Second,
		DisconnectedTimeout: 5 * time.Second,
		FailedTimeout:       25 * time.Second,
		KeepaliveInterval:   2 * time.Second,
		CandidateTypes:      []CandidateType{CandidateTypeHost},
	}

	copied := original.Copy()

	// Modify original to ensure copy is independent
	original.NAT1To1IPs[0] = "modified"
	original.NetworkTypes[0] = NetworkTypeTCP4

	// Verify copy is unchanged
	if copied.NAT1To1IPs[0] == "modified" {
		t.Error("copy should not be affected by changes to original")
	}
	if copied.NetworkTypes[0] == NetworkTypeTCP4 {
		t.Error("copy should not be affected by changes to original")
	}
}

func TestICEConfigWithMethods(t *testing.T) {
	config := DefaultICEConfig()

	// Test WithNAT1To1IPs
	config2 := config.WithNAT1To1IPs([]string{"203.0.113.1"})
	if len(config2.NAT1To1IPs) != 1 {
		t.Error("WithNAT1To1IPs failed")
	}
	if len(config.NAT1To1IPs) != 0 {
		t.Error("original should not be modified")
	}

	// Test WithPortRange
	config3 := config.WithPortRange(5000, 6000)
	if config3.UDPPortRange.Min != 5000 || config3.UDPPortRange.Max != 6000 {
		t.Error("WithPortRange failed")
	}

	// Test WithTimeout
	config4 := config.WithTimeout(60 * time.Second)
	if config4.ConnectionTimeout != 60*time.Second {
		t.Error("WithTimeout failed")
	}

	// Test WithIPv6Disabled
	config5 := config.WithIPv6Disabled()
	if !config5.DisableIPv6 {
		t.Error("WithIPv6Disabled should set DisableIPv6 to true")
	}
	if len(config5.NetworkTypes) != 1 || config5.NetworkTypes[0] != NetworkTypeUDP4 {
		t.Error("WithIPv6Disabled should only include UDP4")
	}
}

func TestNetworkTypeString(t *testing.T) {
	tests := []struct {
		nt       NetworkType
		expected string
	}{
		{NetworkTypeUDP4, "udp4"},
		{NetworkTypeUDP6, "udp6"},
		{NetworkTypeTCP4, "tcp4"},
		{NetworkTypeTCP6, "tcp6"},
		{NetworkType(99), "unknown"},
	}

	for _, tt := range tests {
		if tt.nt.String() != tt.expected {
			t.Errorf("NetworkType(%d).String() = %s, want %s", tt.nt, tt.nt.String(), tt.expected)
		}
	}
}

func TestCandidateTypeString(t *testing.T) {
	tests := []struct {
		ct       CandidateType
		expected string
	}{
		{CandidateTypeHost, "host"},
		{CandidateTypeSrflx, "srflx"},
		{CandidateTypePrflx, "prflx"},
		{CandidateTypeRelay, "relay"},
		{CandidateType(99), "unknown"},
	}

	for _, tt := range tests {
		if tt.ct.String() != tt.expected {
			t.Errorf("CandidateType(%d).String() = %s, want %s", tt.ct, tt.ct.String(), tt.expected)
		}
	}
}

func TestIsICELiteCompatible(t *testing.T) {
	tests := []struct {
		sdp      string
		expected bool
	}{
		{
			sdp:      "v=0\r\na=ice-lite\r\n",
			expected: true,
		},
		{
			sdp:      "v=0\r\na=group:BUNDLE\r\n",
			expected: false,
		},
		{
			sdp:      "",
			expected: false,
		},
	}

	for _, tt := range tests {
		result := IsICELiteCompatible(tt.sdp)
		if result != tt.expected {
			t.Errorf("IsICELiteCompatible(%q) = %v, want %v", tt.sdp, result, tt.expected)
		}
	}
}

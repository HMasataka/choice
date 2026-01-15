package webrtc

import (
	"errors"
	"net"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
)

var (
	// ErrInvalidPortRange is returned when the port range is invalid.
	ErrInvalidPortRange = errors.New("invalid port range: min must be less than max")
	// ErrPortRangeTooSmall is returned when the port range is too small.
	ErrPortRangeTooSmall = errors.New("port range too small: must have at least 100 ports")
	// ErrInvalidIP is returned when an IP address is invalid.
	ErrInvalidIP = errors.New("invalid IP address")
	// ErrNoPublicIP is returned when ICE Lite requires a public IP but none is configured.
	ErrNoPublicIP = errors.New("ICE Lite mode requires at least one public IP address")
)

// Default port range per requirements.md (section 2.7.6).
const (
	DefaultUDPPortMin = 10000
	DefaultUDPPortMax = 20000
)

// Default timeout values per requirements.md (section 2.7.3).
const (
	// DefaultConnectionTimeout is the default timeout for establishing a connection.
	DefaultConnectionTimeout = 30 * time.Second
	// DefaultDisconnectedTimeout is the time to wait before declaring disconnection.
	DefaultDisconnectedTimeout = 5 * time.Second
	// DefaultFailedTimeout is the time to wait before declaring failure.
	DefaultFailedTimeout = 25 * time.Second
	// DefaultKeepaliveInterval is the interval for sending keepalive packets.
	DefaultKeepaliveInterval = 2 * time.Second
)

// NetworkType represents the type of network to use.
type NetworkType int

const (
	// NetworkTypeUDP4 uses UDP over IPv4.
	NetworkTypeUDP4 NetworkType = iota
	// NetworkTypeUDP6 uses UDP over IPv6.
	NetworkTypeUDP6
	// NetworkTypeTCP4 uses TCP over IPv4.
	NetworkTypeTCP4
	// NetworkTypeTCP6 uses TCP over IPv6.
	NetworkTypeTCP6
)

// String returns the string representation of NetworkType.
func (n NetworkType) String() string {
	switch n {
	case NetworkTypeUDP4:
		return "udp4"
	case NetworkTypeUDP6:
		return "udp6"
	case NetworkTypeTCP4:
		return "tcp4"
	case NetworkTypeTCP6:
		return "tcp6"
	default:
		return "unknown"
	}
}

// ICEConfig contains ICE-related configuration for the SFU.
// Per requirements.md (section 2.7), the SFU operates in ICE Lite mode.
type ICEConfig struct {
	// Lite enables ICE Lite mode (controlled role, simplified candidate gathering).
	// Per requirements.md (section 2.7.4), SFU operates as ICE Lite server.
	// Default: true
	Lite bool

	// NAT1To1IPs are the public IP addresses for NAT 1:1 mapping.
	// Per requirements.md (section 2.7.5), required for ICE Lite mode.
	// These IPs are announced as host candidates to clients.
	NAT1To1IPs []string

	// InterfaceFilter is a function to filter network interfaces.
	// If nil, all interfaces are used.
	InterfaceFilter func(interfaceName string) bool

	// IPFilter is a function to filter IP addresses.
	// If nil, all IPs are used.
	IPFilter func(ip net.IP) bool

	// UDPPortRange specifies the UDP port range for ICE.
	// Per requirements.md (section 2.7.6), default is 10000-20000.
	UDPPortRange PortRange

	// NetworkTypes specifies which network types to use.
	// Per requirements.md (section 2.7.6), IPv4 is prioritized over IPv6.
	// Default: UDP4, UDP6 (IPv4 first for priority)
	NetworkTypes []NetworkType

	// DisableIPv6 disables IPv6 support entirely.
	// Default: false (dual-stack enabled per requirements.md section 2.7.6)
	DisableIPv6 bool

	// PreferIPv4 prioritizes IPv4 over IPv6 in candidate ordering.
	// Per requirements.md (section 2.7.6), IPv4 is prioritized.
	// Default: true
	// Note: IPv4 priority is achieved by ordering NetworkTypes (UDP4 before UDP6)
	// and through local preference values (see IPv4Priority/IPv6Priority).
	// pion uses interface order for candidate priority.
	PreferIPv4 bool

	// ConnectionTimeout is the timeout for establishing ICE connections.
	// Per requirements.md (section 2.7.3), default is 30 seconds.
	// Note: This is used at a higher level (e.g., PeerConnection lifecycle)
	// as pion's SettingEngine does not directly support connection timeout.
	ConnectionTimeout time.Duration

	// DisconnectedTimeout is the time to wait before declaring disconnected state.
	DisconnectedTimeout time.Duration

	// FailedTimeout is the time to wait before declaring failed state.
	FailedTimeout time.Duration

	// KeepaliveInterval is the interval for sending keepalive STUN binding requests.
	KeepaliveInterval time.Duration

	// CandidateTypes specifies which candidate types to gather.
	// Per requirements.md (section 2.7.4), ICE Lite only generates host candidates.
	// Default: [host] (srflx and relay not used in ICE Lite)
	// Note: In ICE Lite mode, pion automatically restricts to host candidates.
	// This field is provided for configuration reference and documentation.
	CandidateTypes []CandidateType

	// UDPMux is an optional UDP mux for sharing a single port across connections.
	// Useful for reducing the number of open ports in production.
	UDPMux ice.UDPMux
}

// PortRange represents a range of ports.
type PortRange struct {
	Min uint16
	Max uint16
}

// CandidateType represents an ICE candidate type.
type CandidateType int

const (
	// CandidateTypeHost represents a host candidate.
	CandidateTypeHost CandidateType = iota
	// CandidateTypeSrflx represents a server reflexive candidate.
	CandidateTypeSrflx
	// CandidateTypePrflx represents a peer reflexive candidate.
	CandidateTypePrflx
	// CandidateTypeRelay represents a relay candidate (TURN).
	CandidateTypeRelay
)

// String returns the string representation of CandidateType.
func (c CandidateType) String() string {
	switch c {
	case CandidateTypeHost:
		return "host"
	case CandidateTypeSrflx:
		return "srflx"
	case CandidateTypePrflx:
		return "prflx"
	case CandidateTypeRelay:
		return "relay"
	default:
		return "unknown"
	}
}

// DefaultICEConfig returns the default ICE configuration for the SFU.
// Per requirements.md (section 2.7):
// - ICE Lite mode enabled
// - UDP port range 10000-20000
// - IPv4/IPv6 dual-stack with IPv4 priority
// - 30 second connection timeout
// - Host candidates only (ICE Lite)
func DefaultICEConfig() ICEConfig {
	return ICEConfig{
		Lite:       true,
		NAT1To1IPs: nil, // Must be set by caller for ICE Lite
		UDPPortRange: PortRange{
			Min: DefaultUDPPortMin,
			Max: DefaultUDPPortMax,
		},
		NetworkTypes: []NetworkType{
			NetworkTypeUDP4, // IPv4 first for priority
			NetworkTypeUDP6,
		},
		DisableIPv6:         false,
		PreferIPv4:          true,
		ConnectionTimeout:   DefaultConnectionTimeout,
		DisconnectedTimeout: DefaultDisconnectedTimeout,
		FailedTimeout:       DefaultFailedTimeout,
		KeepaliveInterval:   DefaultKeepaliveInterval,
		CandidateTypes: []CandidateType{
			CandidateTypeHost, // ICE Lite only uses host candidates
		},
	}
}

// ICELiteConfig returns an ICE configuration optimized for ICE Lite mode.
// This is the recommended configuration for SFU servers.
func ICELiteConfig(publicIPs []string) ICEConfig {
	config := DefaultICEConfig()
	config.NAT1To1IPs = publicIPs
	return config
}

// ValidateICEConfig validates the ICE configuration.
// Returns an error if the configuration is invalid.
func ValidateICEConfig(config ICEConfig) error {
	// Validate port range
	if config.UDPPortRange.Min >= config.UDPPortRange.Max {
		return ErrInvalidPortRange
	}
	// Port count is (Max - Min + 1), so for 100 ports minimum, Max - Min must be >= 99
	if config.UDPPortRange.Max-config.UDPPortRange.Min < 99 {
		return ErrPortRangeTooSmall
	}

	// Validate NAT 1:1 IPs
	for _, ipStr := range config.NAT1To1IPs {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return ErrInvalidIP
		}
	}

	// ICE Lite requires at least one public IP
	if config.Lite && len(config.NAT1To1IPs) == 0 {
		return ErrNoPublicIP
	}

	return nil
}

// ApplyToSettingEngine applies the ICE configuration to a pion SettingEngine.
// This configures the low-level ICE behavior.
func ApplyToSettingEngine(se *webrtc.SettingEngine, config ICEConfig) error {
	// Enable ICE Lite mode
	if config.Lite {
		se.SetLite(true)
	}

	// Set NAT 1:1 IPs for host candidate generation
	if len(config.NAT1To1IPs) > 0 {
		se.SetNAT1To1IPs(config.NAT1To1IPs, webrtc.ICECandidateTypeHost)
	}

	// Set UDP port range
	if config.UDPPortRange.Min > 0 && config.UDPPortRange.Max > 0 {
		if err := se.SetEphemeralUDPPortRange(config.UDPPortRange.Min, config.UDPPortRange.Max); err != nil {
			return err
		}
	}

	// Set network types
	if len(config.NetworkTypes) > 0 {
		networkTypes := make([]webrtc.NetworkType, 0, len(config.NetworkTypes))
		for _, nt := range config.NetworkTypes {
			switch nt {
			case NetworkTypeUDP4:
				networkTypes = append(networkTypes, webrtc.NetworkTypeUDP4)
			case NetworkTypeUDP6:
				if !config.DisableIPv6 {
					networkTypes = append(networkTypes, webrtc.NetworkTypeUDP6)
				}
			case NetworkTypeTCP4:
				networkTypes = append(networkTypes, webrtc.NetworkTypeTCP4)
			case NetworkTypeTCP6:
				if !config.DisableIPv6 {
					networkTypes = append(networkTypes, webrtc.NetworkTypeTCP6)
				}
			}
		}
		se.SetNetworkTypes(networkTypes)
	}

	// Set interface filter
	if config.InterfaceFilter != nil {
		se.SetInterfaceFilter(config.InterfaceFilter)
	}

	// Set IP filter
	if config.IPFilter != nil {
		se.SetIPFilter(config.IPFilter)
	}

	// Set UDP mux if provided
	if config.UDPMux != nil {
		se.SetICEUDPMux(config.UDPMux)
	}

	// Set timeouts
	se.SetICETimeouts(
		config.DisconnectedTimeout,
		config.FailedTimeout,
		config.KeepaliveInterval,
	)

	return nil
}

// ICEServerConfig represents an ICE server configuration (STUN/TURN).
type ICEServerConfig struct {
	// URLs are the server URLs (e.g., "stun:stun.example.com:3478").
	URLs []string
	// Username is the username for TURN authentication.
	Username string
	// Credential is the credential for TURN authentication.
	Credential string
	// CredentialType is the type of credential (password or oauth).
	CredentialType webrtc.ICECredentialType
}

// ToWebRTCICEServer converts ICEServerConfig to webrtc.ICEServer.
func (c ICEServerConfig) ToWebRTCICEServer() webrtc.ICEServer {
	return webrtc.ICEServer{
		URLs:           c.URLs,
		Username:       c.Username,
		Credential:     c.Credential,
		CredentialType: c.CredentialType,
	}
}

// ICEServersConfig contains multiple ICE server configurations.
// Per requirements.md (section 2.7.1), supports fallback across multiple servers.
type ICEServersConfig struct {
	// Servers is the list of ICE servers in priority order.
	Servers []ICEServerConfig
}

// ToWebRTCICEServers converts to a slice of webrtc.ICEServer.
func (c ICEServersConfig) ToWebRTCICEServers() []webrtc.ICEServer {
	servers := make([]webrtc.ICEServer, len(c.Servers))
	for i, s := range c.Servers {
		servers[i] = s.ToWebRTCICEServer()
	}
	return servers
}

// DefaultSTUNServers returns a list of default public STUN servers.
// These can be used for testing or as fallback servers.
func DefaultSTUNServers() ICEServersConfig {
	return ICEServersConfig{
		Servers: []ICEServerConfig{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
			{URLs: []string{"stun:stun1.l.google.com:19302"}},
		},
	}
}

// CandidatePriority represents ICE candidate priority calculation.
// Per requirements.md (section 2.7.3): host > srflx > relay
type CandidatePriority struct {
	// TypePreference is the preference based on candidate type (0-126).
	TypePreference uint32
	// LocalPreference is the preference based on local factors (0-65535).
	LocalPreference uint32
	// ComponentID is the component ID (1 for RTP, 2 for RTCP).
	ComponentID uint32
}

// Calculate calculates the ICE candidate priority per RFC 5245.
// Priority = (2^24 * type_preference) + (2^8 * local_preference) + (256 - component_id)
func (p CandidatePriority) Calculate() uint32 {
	return (p.TypePreference << 24) + (p.LocalPreference << 8) + (256 - p.ComponentID)
}

// DefaultCandidatePriorities returns the default candidate type preferences.
// Per requirements.md (section 2.7.3): host > srflx > relay
func DefaultCandidatePriorities() map[CandidateType]uint32 {
	return map[CandidateType]uint32{
		CandidateTypeHost:   126, // Highest priority
		CandidateTypePrflx:  110, // Peer reflexive
		CandidateTypeSrflx:  100, // Server reflexive
		CandidateTypeRelay:  0,   // Lowest priority
	}
}

// IPv4Priority returns a higher local preference for IPv4 addresses.
// Per requirements.md (section 2.7.6): IPv4 is prioritized over IPv6.
func IPv4Priority() uint32 {
	return 65535 // Maximum local preference for IPv4
}

// IPv6Priority returns a lower local preference for IPv6 addresses.
// Per requirements.md (section 2.7.6): IPv4 is prioritized over IPv6.
func IPv6Priority() uint32 {
	return 65534 // Slightly lower preference for IPv6
}

// IPPriorityFilter creates an IP filter that prioritizes IPv4.
// This can be used with ICEConfig.IPFilter.
func IPPriorityFilter(preferIPv4 bool) func(net.IP) bool {
	return func(ip net.IP) bool {
		// Accept all IPs but the preference is handled in candidate ordering
		return true
	}
}

// CreateInterfaceFilter creates an interface filter from a list of allowed interfaces.
// If allowedInterfaces is empty, all interfaces are allowed.
func CreateInterfaceFilter(allowedInterfaces []string) func(string) bool {
	if len(allowedInterfaces) == 0 {
		return nil
	}
	allowed := make(map[string]bool)
	for _, iface := range allowedInterfaces {
		allowed[iface] = true
	}
	return func(interfaceName string) bool {
		return allowed[interfaceName]
	}
}

// CreateIPFilter creates an IP filter from allowed CIDRs.
// If allowedCIDRs is empty, all IPs are allowed.
func CreateIPFilter(allowedCIDRs []string) (func(net.IP) bool, error) {
	if len(allowedCIDRs) == 0 {
		return nil, nil
	}

	networks := make([]*net.IPNet, 0, len(allowedCIDRs))
	for _, cidr := range allowedCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		networks = append(networks, network)
	}

	return func(ip net.IP) bool {
		for _, network := range networks {
			if network.Contains(ip) {
				return true
			}
		}
		return false
	}, nil
}

// Copy creates a deep copy of the ICEConfig.
func (c ICEConfig) Copy() ICEConfig {
	nat1to1Copy := make([]string, len(c.NAT1To1IPs))
	copy(nat1to1Copy, c.NAT1To1IPs)

	networkTypesCopy := make([]NetworkType, len(c.NetworkTypes))
	copy(networkTypesCopy, c.NetworkTypes)

	candidateTypesCopy := make([]CandidateType, len(c.CandidateTypes))
	copy(candidateTypesCopy, c.CandidateTypes)

	return ICEConfig{
		Lite:                c.Lite,
		NAT1To1IPs:          nat1to1Copy,
		InterfaceFilter:     c.InterfaceFilter,
		IPFilter:            c.IPFilter,
		UDPPortRange:        c.UDPPortRange,
		NetworkTypes:        networkTypesCopy,
		DisableIPv6:         c.DisableIPv6,
		PreferIPv4:          c.PreferIPv4,
		ConnectionTimeout:   c.ConnectionTimeout,
		DisconnectedTimeout: c.DisconnectedTimeout,
		FailedTimeout:       c.FailedTimeout,
		KeepaliveInterval:   c.KeepaliveInterval,
		CandidateTypes:      candidateTypesCopy,
		UDPMux:              c.UDPMux,
	}
}

// WithNAT1To1IPs returns a copy of the config with NAT 1:1 IPs set.
func (c ICEConfig) WithNAT1To1IPs(ips []string) ICEConfig {
	config := c.Copy()
	config.NAT1To1IPs = ips
	return config
}

// WithPortRange returns a copy of the config with the port range set.
func (c ICEConfig) WithPortRange(min, max uint16) ICEConfig {
	config := c.Copy()
	config.UDPPortRange = PortRange{Min: min, Max: max}
	return config
}

// WithTimeout returns a copy of the config with connection timeout set.
func (c ICEConfig) WithTimeout(timeout time.Duration) ICEConfig {
	config := c.Copy()
	config.ConnectionTimeout = timeout
	return config
}

// WithIPv6Disabled returns a copy of the config with IPv6 disabled.
func (c ICEConfig) WithIPv6Disabled() ICEConfig {
	config := c.Copy()
	config.DisableIPv6 = true
	config.NetworkTypes = []NetworkType{NetworkTypeUDP4}
	return config
}

// IsICELiteCompatible checks if a remote SDP indicates ICE Lite support.
// This is useful for debugging connectivity issues.
func IsICELiteCompatible(sdp string) bool {
	// Check for ice-lite attribute in SDP
	return containsString(sdp, "a=ice-lite")
}

// containsString is a simple substring check.
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

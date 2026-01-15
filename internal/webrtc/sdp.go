package webrtc

import (
	"errors"
	"regexp"
	"strings"

	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
)

var (
	// ErrInvalidSDPFormat is returned when the SDP format is invalid.
	ErrInvalidSDPFormat = errors.New("invalid SDP format")
	// ErrUnsupportedSDPSemantics is returned when Plan B semantics are detected.
	ErrUnsupportedSDPSemantics = errors.New("Plan B semantics not supported, use Unified Plan")
	// ErrMissingBundle is returned when BUNDLE is not present.
	ErrMissingBundle = errors.New("BUNDLE is required")
	// ErrMissingRTCPMux is returned when rtcp-mux is not present.
	ErrMissingRTCPMux = errors.New("rtcp-mux is required")
)

// SDPConfig contains configuration for SDP processing.
type SDPConfig struct {
	// EnableSafariCompat enables Safari compatibility mode.
	EnableSafariCompat bool
}

// DefaultSDPConfig returns a default SDPConfig.
func DefaultSDPConfig() SDPConfig {
	return SDPConfig{
		EnableSafariCompat: true,
	}
}

// SDPProcessor handles SDP parsing, generation, and normalization.
type SDPProcessor struct {
	config SDPConfig
}

// NewSDPProcessor creates a new SDPProcessor with the given configuration.
func NewSDPProcessor(config SDPConfig) *SDPProcessor {
	return &SDPProcessor{
		config: config,
	}
}

// ParseSDP parses an SDP string into a SessionDescription.
func (p *SDPProcessor) ParseSDP(sdpStr string) (*sdp.SessionDescription, error) {
	if sdpStr == "" {
		return nil, ErrInvalidSDPFormat
	}

	sd := &sdp.SessionDescription{}
	if err := sd.Unmarshal([]byte(sdpStr)); err != nil {
		return nil, err
	}

	return sd, nil
}

// ValidateOffer validates an SDP offer for required features.
func (p *SDPProcessor) ValidateOffer(offer webrtc.SessionDescription) error {
	if offer.Type != webrtc.SDPTypeOffer {
		return ErrInvalidSDPFormat
	}

	sd, err := p.ParseSDP(offer.SDP)
	if err != nil {
		return err
	}

	return p.validateSDP(sd)
}

// ValidateAnswer validates an SDP answer for required features.
func (p *SDPProcessor) ValidateAnswer(answer webrtc.SessionDescription) error {
	if answer.Type != webrtc.SDPTypeAnswer {
		return ErrInvalidSDPFormat
	}

	sd, err := p.ParseSDP(answer.SDP)
	if err != nil {
		return err
	}

	return p.validateSDP(sd)
}

// validateSDP validates an SDP for required features.
func (p *SDPProcessor) validateSDP(sd *sdp.SessionDescription) error {
	// Check for BUNDLE
	if !p.hasBundle(sd) {
		return ErrMissingBundle
	}

	// Check for rtcp-mux in all media sections
	if !p.hasRTCPMux(sd) {
		return ErrMissingRTCPMux
	}

	// Enforce Unified Plan semantics (each m= line must have a unique mid)
	if err := p.enforceUnifiedPlan(sd); err != nil {
		return err
	}

	return nil
}

// hasBundle checks if the SDP contains a BUNDLE group.
func (p *SDPProcessor) hasBundle(sd *sdp.SessionDescription) bool {
	for _, attr := range sd.Attributes {
		if attr.Key == "group" && strings.HasPrefix(attr.Value, "BUNDLE") {
			return true
		}
	}
	return false
}

// hasRTCPMux checks if all media sections have rtcp-mux.
func (p *SDPProcessor) hasRTCPMux(sd *sdp.SessionDescription) bool {
	for _, media := range sd.MediaDescriptions {
		hasRTCPMux := false
		for _, attr := range media.Attributes {
			if attr.Key == "rtcp-mux" {
				hasRTCPMux = true
				break
			}
		}
		if !hasRTCPMux {
			return false
		}
	}
	return true
}

// enforceUnifiedPlan ensures all media sections have unique MID values.
// This is a requirement for Unified Plan semantics.
func (p *SDPProcessor) enforceUnifiedPlan(sd *sdp.SessionDescription) error {
	if len(sd.MediaDescriptions) == 0 {
		return nil
	}

	mids := make(map[string]bool)
	for i, media := range sd.MediaDescriptions {
		mid := ""
		for _, attr := range media.Attributes {
			if attr.Key == "mid" {
				mid = attr.Value
				break
			}
		}

		// Each m= line must have a mid in Unified Plan
		if mid == "" {
			// Allow missing mid only for rejected (port=0) media sections
			if media.MediaName.Port.Value != 0 {
				return ErrUnsupportedSDPSemantics
			}
			continue
		}

		// Check for duplicate MIDs
		if mids[mid] {
			return ErrUnsupportedSDPSemantics
		}
		mids[mid] = true

		// In Unified Plan, each m= line has at most one track (one SSRC group)
		// Check for signs of Plan B (multiple unrelated SSRCs)
		ssrcGroups := p.countSSRCGroups(media, i)
		if ssrcGroups > 1 {
			return ErrUnsupportedSDPSemantics
		}
	}

	return nil
}

// countSSRCGroups counts the number of SSRC groups in a media section.
// In Unified Plan, there should be at most one group per m= line.
func (p *SDPProcessor) countSSRCGroups(media *sdp.MediaDescription, _ int) int {
	// Collect SSRCs
	ssrcs := make(map[string]bool)
	groupedSSRCs := make(map[string]bool)

	for _, attr := range media.Attributes {
		if attr.Key == "ssrc" {
			parts := strings.Fields(attr.Value)
			if len(parts) > 0 {
				ssrcs[parts[0]] = true
			}
		}
		// ssrc-group bundles related SSRCs (e.g., FID for RTX, SIM for simulcast)
		if attr.Key == "ssrc-group" {
			parts := strings.Fields(attr.Value)
			for i := 1; i < len(parts); i++ {
				groupedSSRCs[parts[i]] = true
			}
		}
	}

	// Count ungrouped SSRCs (each represents a potential separate track)
	ungroupedCount := 0
	for ssrc := range ssrcs {
		if !groupedSSRCs[ssrc] {
			ungroupedCount++
		}
	}

	// If all SSRCs are grouped or there's 0-1 ungrouped, it's valid
	if ungroupedCount <= 1 {
		return 1
	}

	// Multiple ungrouped SSRCs suggest Plan B
	return ungroupedCount
}

// NormalizeForSafari normalizes an SDP for Safari compatibility.
func (p *SDPProcessor) NormalizeForSafari(sdpStr string) (string, error) {
	if !p.config.EnableSafariCompat {
		return sdpStr, nil
	}

	// Safari-specific SDP adjustments
	normalized := sdpStr

	// Ensure proper line endings (CRLF)
	normalized = strings.ReplaceAll(normalized, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\n", "\r\n")

	// Safari requires specific ordering of attributes
	// This is handled by pion internally, but we ensure consistency

	return normalized, nil
}

// EnsureUnifiedPlan ensures the SDP uses Unified Plan semantics.
// This is an alias for enforceUnifiedPlan for external callers.
func (p *SDPProcessor) EnsureUnifiedPlan(sd *sdp.SessionDescription) error {
	return p.enforceUnifiedPlan(sd)
}

// ExtractMIDs extracts all MID values from the SDP.
func (p *SDPProcessor) ExtractMIDs(sdpStr string) ([]string, error) {
	sd, err := p.ParseSDP(sdpStr)
	if err != nil {
		return nil, err
	}

	var mids []string
	for _, media := range sd.MediaDescriptions {
		for _, attr := range media.Attributes {
			if attr.Key == "mid" {
				mids = append(mids, attr.Value)
				break
			}
		}
	}

	return mids, nil
}

// ExtractCodecs extracts codec information from the SDP.
func (p *SDPProcessor) ExtractCodecs(sdpStr string) ([]CodecInfo, error) {
	sd, err := p.ParseSDP(sdpStr)
	if err != nil {
		return nil, err
	}

	var codecs []CodecInfo
	for _, media := range sd.MediaDescriptions {
		mediaType := media.MediaName.Media
		for _, format := range media.MediaName.Formats {
			codec := CodecInfo{
				PayloadType: format,
				MediaType:   mediaType,
			}

			// Find rtpmap for this payload type
			for _, attr := range media.Attributes {
				if attr.Key == "rtpmap" && strings.HasPrefix(attr.Value, format+" ") {
					parts := strings.SplitN(attr.Value, " ", 2)
					if len(parts) == 2 {
						codec.Name = strings.Split(parts[1], "/")[0]
					}
				}
				if attr.Key == "fmtp" && strings.HasPrefix(attr.Value, format+" ") {
					parts := strings.SplitN(attr.Value, " ", 2)
					if len(parts) == 2 {
						codec.FMTPParams = parts[1]
					}
				}
			}

			codecs = append(codecs, codec)
		}
	}

	return codecs, nil
}

// CodecInfo contains information about a codec in the SDP.
type CodecInfo struct {
	PayloadType string
	MediaType   string
	Name        string
	ClockRate   int
	FMTPParams  string
}

// GetBundleGroup extracts the BUNDLE group MIDs from the SDP.
func (p *SDPProcessor) GetBundleGroup(sdpStr string) ([]string, error) {
	sd, err := p.ParseSDP(sdpStr)
	if err != nil {
		return nil, err
	}

	for _, attr := range sd.Attributes {
		if attr.Key == "group" && strings.HasPrefix(attr.Value, "BUNDLE") {
			parts := strings.Fields(attr.Value)
			if len(parts) > 1 {
				return parts[1:], nil
			}
		}
	}

	return nil, ErrMissingBundle
}

// IsSimulcastEnabled checks if simulcast is enabled in the SDP.
func (p *SDPProcessor) IsSimulcastEnabled(sdpStr string) (bool, error) {
	sd, err := p.ParseSDP(sdpStr)
	if err != nil {
		return false, err
	}

	for _, media := range sd.MediaDescriptions {
		for _, attr := range media.Attributes {
			if attr.Key == "simulcast" {
				return true, nil
			}
			// Also check for rid attributes which indicate simulcast layers
			if attr.Key == "rid" {
				return true, nil
			}
		}
	}

	return false, nil
}

// ExtractSimulcastLayers extracts simulcast layer information from the SDP.
func (p *SDPProcessor) ExtractSimulcastLayers(sdpStr string) ([]SimulcastLayer, error) {
	sd, err := p.ParseSDP(sdpStr)
	if err != nil {
		return nil, err
	}

	var layers []SimulcastLayer
	for _, media := range sd.MediaDescriptions {
		for _, attr := range media.Attributes {
			if attr.Key == "rid" {
				parts := strings.Fields(attr.Value)
				if len(parts) >= 2 {
					layer := SimulcastLayer{
						RID:       parts[0],
						Direction: parts[1],
					}
					layers = append(layers, layer)
				}
			}
		}
	}

	return layers, nil
}

// SimulcastLayer represents a simulcast layer in the SDP.
type SimulcastLayer struct {
	RID       string
	Direction string
}

// ModifySDP allows modification of an SDP using a callback function.
func (p *SDPProcessor) ModifySDP(sdpStr string, modifier func(*sdp.SessionDescription) error) (string, error) {
	sd, err := p.ParseSDP(sdpStr)
	if err != nil {
		return "", err
	}

	if err := modifier(sd); err != nil {
		return "", err
	}

	result, err := sd.Marshal()
	if err != nil {
		return "", err
	}

	return string(result), nil
}

// RemoveCodec removes a codec from the SDP by name.
func (p *SDPProcessor) RemoveCodec(sdpStr string, codecName string) (string, error) {
	return p.ModifySDP(sdpStr, func(sd *sdp.SessionDescription) error {
		for _, media := range sd.MediaDescriptions {
			var newFormats []string
			var payloadTypesToRemove []string

			// Find payload types for the codec to remove
			for _, attr := range media.Attributes {
				if attr.Key == "rtpmap" {
					if strings.Contains(strings.ToLower(attr.Value), strings.ToLower(codecName)) {
						parts := strings.SplitN(attr.Value, " ", 2)
						if len(parts) > 0 {
							payloadTypesToRemove = append(payloadTypesToRemove, parts[0])
						}
					}
				}
			}

			// Filter formats
			for _, format := range media.MediaName.Formats {
				remove := false
				for _, pt := range payloadTypesToRemove {
					if format == pt {
						remove = true
						break
					}
				}
				if !remove {
					newFormats = append(newFormats, format)
				}
			}
			media.MediaName.Formats = newFormats

			// Filter attributes
			var newAttrs []sdp.Attribute
			for _, attr := range media.Attributes {
				keep := true
				for _, pt := range payloadTypesToRemove {
					if (attr.Key == "rtpmap" || attr.Key == "fmtp" || attr.Key == "rtcp-fb") &&
						strings.HasPrefix(attr.Value, pt+" ") {
						keep = false
						break
					}
				}
				if keep {
					newAttrs = append(newAttrs, attr)
				}
			}
			media.Attributes = newAttrs
		}
		return nil
	})
}

// SetCodecPriority reorders codecs in the SDP by priority.
func (p *SDPProcessor) SetCodecPriority(sdpStr string, mediaType string, priorities []string) (string, error) {
	return p.ModifySDP(sdpStr, func(sd *sdp.SessionDescription) error {
		for _, media := range sd.MediaDescriptions {
			if media.MediaName.Media != mediaType {
				continue
			}

			// Map codec names to payload types
			codecToPayload := make(map[string]string)
			for _, attr := range media.Attributes {
				if attr.Key == "rtpmap" {
					parts := strings.SplitN(attr.Value, " ", 2)
					if len(parts) == 2 {
						pt := parts[0]
						codecName := strings.Split(parts[1], "/")[0]
						codecToPayload[strings.ToLower(codecName)] = pt
					}
				}
			}

			// Build new format list based on priorities
			var newFormats []string
			usedPayloads := make(map[string]bool)

			// Add prioritized codecs first
			for _, priority := range priorities {
				if pt, ok := codecToPayload[strings.ToLower(priority)]; ok {
					if !usedPayloads[pt] {
						newFormats = append(newFormats, pt)
						usedPayloads[pt] = true
					}
				}
			}

			// Add remaining codecs
			for _, format := range media.MediaName.Formats {
				if !usedPayloads[format] {
					newFormats = append(newFormats, format)
				}
			}

			media.MediaName.Formats = newFormats
		}
		return nil
	})
}

// AddAttribute adds an attribute to the SDP at session level.
func (p *SDPProcessor) AddAttribute(sdpStr string, key string, value string) (string, error) {
	return p.ModifySDP(sdpStr, func(sd *sdp.SessionDescription) error {
		sd.Attributes = append(sd.Attributes, sdp.Attribute{
			Key:   key,
			Value: value,
		})
		return nil
	})
}

// GetICECredentials extracts ICE credentials from the SDP.
func (p *SDPProcessor) GetICECredentials(sdpStr string) (ufrag string, pwd string, err error) {
	sd, err := p.ParseSDP(sdpStr)
	if err != nil {
		return "", "", err
	}

	// Check session-level attributes first
	for _, attr := range sd.Attributes {
		switch attr.Key {
		case "ice-ufrag":
			ufrag = attr.Value
		case "ice-pwd":
			pwd = attr.Value
		}
	}

	// If not found at session level, check media level
	if ufrag == "" || pwd == "" {
		for _, media := range sd.MediaDescriptions {
			for _, attr := range media.Attributes {
				switch attr.Key {
				case "ice-ufrag":
					if ufrag == "" {
						ufrag = attr.Value
					}
				case "ice-pwd":
					if pwd == "" {
						pwd = attr.Value
					}
				}
			}
			if ufrag != "" && pwd != "" {
				break
			}
		}
	}

	return ufrag, pwd, nil
}

// GetFingerprint extracts the DTLS fingerprint from the SDP.
func (p *SDPProcessor) GetFingerprint(sdpStr string) (algorithm string, fingerprint string, err error) {
	sd, err := p.ParseSDP(sdpStr)
	if err != nil {
		return "", "", err
	}

	// Check session-level attributes first
	for _, attr := range sd.Attributes {
		if attr.Key == "fingerprint" {
			parts := strings.SplitN(attr.Value, " ", 2)
			if len(parts) == 2 {
				return parts[0], parts[1], nil
			}
		}
	}

	// Check media level
	for _, media := range sd.MediaDescriptions {
		for _, attr := range media.Attributes {
			if attr.Key == "fingerprint" {
				parts := strings.SplitN(attr.Value, " ", 2)
				if len(parts) == 2 {
					return parts[0], parts[1], nil
				}
			}
		}
	}

	return "", "", nil
}

// reSSRC is a regex to match SSRC attributes.
var reSSRC = regexp.MustCompile(`^(\d+)`)

// ExtractSSRCs extracts all SSRCs from the SDP.
func (p *SDPProcessor) ExtractSSRCs(sdpStr string) ([]uint32, error) {
	sd, err := p.ParseSDP(sdpStr)
	if err != nil {
		return nil, err
	}

	ssrcMap := make(map[uint32]bool)
	for _, media := range sd.MediaDescriptions {
		for _, attr := range media.Attributes {
			if attr.Key == "ssrc" {
				matches := reSSRC.FindStringSubmatch(attr.Value)
				if len(matches) > 1 {
					var ssrc uint32
					if _, err := parseUint32(matches[1], &ssrc); err == nil {
						ssrcMap[ssrc] = true
					}
				}
			}
		}
	}

	var ssrcs []uint32
	for ssrc := range ssrcMap {
		ssrcs = append(ssrcs, ssrc)
	}

	return ssrcs, nil
}

// parseUint32 parses a string to uint32.
func parseUint32(s string, result *uint32) (bool, error) {
	var val uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			return false, errors.New("invalid character")
		}
		val = val*10 + uint64(c-'0')
		if val > 0xFFFFFFFF {
			return false, errors.New("overflow")
		}
	}
	*result = uint32(val)
	return true, nil
}

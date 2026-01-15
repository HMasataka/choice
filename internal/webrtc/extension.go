package webrtc

import (
	"errors"

	"github.com/pion/webrtc/v4"
)

var (
	// ErrDuplicateExtensionID is returned when duplicate extension IDs are found.
	ErrDuplicateExtensionID = errors.New("duplicate extension ID")
	// ErrInvalidExtensionID is returned when the extension ID is out of valid range.
	// Per RFC 8285, one-byte header extension IDs must be 1-14 (0 and 15 are reserved).
	ErrInvalidExtensionID = errors.New("extension ID must be between 1 and 14")
	// ErrEmptyExtensionURI is returned when the extension URI is empty.
	ErrEmptyExtensionURI = errors.New("extension URI cannot be empty")
	// ErrDuplicateExtensionURI is returned when duplicate extension URIs are found within a media type.
	ErrDuplicateExtensionURI = errors.New("duplicate extension URI")
)

// RTP Header Extension URIs per requirements.md (section 2.4.2) and design.md (section 3.6.2).
const (
	// ExtensionURIMID is the URI for the MID (Media ID) extension.
	// Used to identify media sections in Unified Plan SDP.
	// See RFC 8843 (BUNDLE) and RFC 8851 (RID-based Simulcast).
	ExtensionURIMID = "urn:ietf:params:rtp-hdrext:sdes:mid"

	// ExtensionURIRID is the URI for the RID (Restriction ID) extension.
	// Used to identify simulcast layers.
	// See RFC 8851 (RTP Stream Identifier).
	ExtensionURIRID = "urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id"

	// ExtensionURIRepairedRID is the URI for the repaired RID extension.
	// Used to identify repaired streams in RTX (retransmission).
	ExtensionURIRepairedRID = "urn:ietf:params:rtp-hdrext:sdes:repaired-rtp-stream-id"

	// ExtensionURITransportCC is the URI for transport-wide congestion control.
	// Used for TWCC (Transport-Wide Congestion Control).
	// See draft-holmer-rmcat-transport-wide-cc-extensions.
	ExtensionURITransportCC = "http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01"

	// ExtensionURIAbsSendTime is the URI for absolute send time extension.
	// Used for measuring one-way delay and congestion control.
	ExtensionURIAbsSendTime = "http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time"

	// ExtensionURIAudioLevel is the URI for audio level indication.
	// Used for voice activity detection and audio mixing.
	ExtensionURIAudioLevel = "urn:ietf:params:rtp-hdrext:ssrc-audio-level"

	// ExtensionURIVideoOrientation is the URI for video orientation.
	// Used for coordinating video rotation between sender and receiver.
	ExtensionURIVideoOrientation = "urn:3gpp:video-orientation"
)

// ExtensionID represents a header extension ID (1-15 for one-byte form).
type ExtensionID int

// Standard extension IDs.
// Note: These IDs can be negotiated during SDP exchange, but we define
// preferred defaults that align with common implementations.
const (
	// ExtensionIDMID is the default ID for MID extension.
	ExtensionIDMID ExtensionID = 1
	// ExtensionIDRID is the default ID for RID extension.
	ExtensionIDRID ExtensionID = 2
	// ExtensionIDRepairedRID is the default ID for repaired RID extension.
	ExtensionIDRepairedRID ExtensionID = 3
	// ExtensionIDAbsSendTime is the default ID for abs-send-time extension.
	ExtensionIDAbsSendTime ExtensionID = 4
	// ExtensionIDTransportCC is the default ID for transport-cc extension.
	ExtensionIDTransportCC ExtensionID = 5
	// ExtensionIDAudioLevel is the default ID for audio level extension.
	ExtensionIDAudioLevel ExtensionID = 6
	// ExtensionIDVideoOrientation is the default ID for video orientation.
	ExtensionIDVideoOrientation ExtensionID = 7
)

// HeaderExtension represents an RTP header extension configuration.
type HeaderExtension struct {
	// ID is the preferred extension ID (1-15 for one-byte form).
	// Note: This ID is used for configuration validation only.
	// pion/webrtc negotiates actual IDs during SDP exchange.
	// The actual negotiated ID may differ from this preferred value.
	ID ExtensionID
	// URI is the extension URI that identifies the extension type.
	// This is the primary identifier used by pion/webrtc for registration.
	URI string
	// Direction specifies when this extension is used.
	Direction ExtensionDirection
}

// ExtensionDirection specifies when a header extension is used.
type ExtensionDirection int

const (
	// ExtensionDirectionSendRecv indicates the extension is used in both directions.
	ExtensionDirectionSendRecv ExtensionDirection = iota
	// ExtensionDirectionSendOnly indicates the extension is used only for sending.
	ExtensionDirectionSendOnly
	// ExtensionDirectionRecvOnly indicates the extension is used only for receiving.
	ExtensionDirectionRecvOnly
)

// HeaderExtensionConfig contains the configuration for RTP header extensions.
type HeaderExtensionConfig struct {
	// VideoExtensions are the extensions to register for video.
	VideoExtensions []HeaderExtension
	// AudioExtensions are the extensions to register for audio.
	AudioExtensions []HeaderExtension
}

// DefaultHeaderExtensionConfig returns the default header extension configuration.
// Per requirements.md (section 2.4.2) and design.md (section 3.6.2):
// Required extensions: mid, rid, transport-wide-cc, abs-send-time
func DefaultHeaderExtensionConfig() HeaderExtensionConfig {
	return HeaderExtensionConfig{
		VideoExtensions: []HeaderExtension{
			{
				ID:        ExtensionIDMID,
				URI:       ExtensionURIMID,
				Direction: ExtensionDirectionSendRecv,
			},
			{
				ID:        ExtensionIDRID,
				URI:       ExtensionURIRID,
				Direction: ExtensionDirectionSendRecv,
			},
			{
				ID:        ExtensionIDRepairedRID,
				URI:       ExtensionURIRepairedRID,
				Direction: ExtensionDirectionSendRecv,
			},
			{
				ID:        ExtensionIDAbsSendTime,
				URI:       ExtensionURIAbsSendTime,
				Direction: ExtensionDirectionSendRecv,
			},
			{
				ID:        ExtensionIDTransportCC,
				URI:       ExtensionURITransportCC,
				Direction: ExtensionDirectionSendRecv,
			},
			{
				ID:        ExtensionIDVideoOrientation,
				URI:       ExtensionURIVideoOrientation,
				Direction: ExtensionDirectionSendRecv,
			},
		},
		AudioExtensions: []HeaderExtension{
			{
				ID:        ExtensionIDMID,
				URI:       ExtensionURIMID,
				Direction: ExtensionDirectionSendRecv,
			},
			{
				ID:        ExtensionIDAbsSendTime,
				URI:       ExtensionURIAbsSendTime,
				Direction: ExtensionDirectionSendRecv,
			},
			{
				ID:        ExtensionIDTransportCC,
				URI:       ExtensionURITransportCC,
				Direction: ExtensionDirectionSendRecv,
			},
			{
				ID:        ExtensionIDAudioLevel,
				URI:       ExtensionURIAudioLevel,
				Direction: ExtensionDirectionSendRecv,
			},
		},
	}
}

// MinimalHeaderExtensionConfig returns a minimal header extension configuration.
// This includes only the strictly required extensions per spec.
func MinimalHeaderExtensionConfig() HeaderExtensionConfig {
	return HeaderExtensionConfig{
		VideoExtensions: []HeaderExtension{
			{
				ID:        ExtensionIDMID,
				URI:       ExtensionURIMID,
				Direction: ExtensionDirectionSendRecv,
			},
			{
				ID:        ExtensionIDRID,
				URI:       ExtensionURIRID,
				Direction: ExtensionDirectionSendRecv,
			},
			{
				ID:        ExtensionIDTransportCC,
				URI:       ExtensionURITransportCC,
				Direction: ExtensionDirectionSendRecv,
			},
			{
				ID:        ExtensionIDAbsSendTime,
				URI:       ExtensionURIAbsSendTime,
				Direction: ExtensionDirectionSendRecv,
			},
		},
		AudioExtensions: []HeaderExtension{
			{
				ID:        ExtensionIDMID,
				URI:       ExtensionURIMID,
				Direction: ExtensionDirectionSendRecv,
			},
			{
				ID:        ExtensionIDTransportCC,
				URI:       ExtensionURITransportCC,
				Direction: ExtensionDirectionSendRecv,
			},
			{
				ID:        ExtensionIDAbsSendTime,
				URI:       ExtensionURIAbsSendTime,
				Direction: ExtensionDirectionSendRecv,
			},
		},
	}
}

// RegisterHeaderExtensions registers all header extensions from the config to the MediaEngine.
// This must be called before creating PeerConnections.
//
// The function registers extensions for both video and audio codecs.
// Extension IDs in the config are used for validation purposes only.
//
// Note: pion/webrtc negotiates actual extension IDs during SDP exchange.
// The IDs specified in HeaderExtension.ID are not directly used by pion;
// instead, they serve as preferred hints and for configuration validation.
// The actual negotiated IDs can be retrieved after SDP exchange using
// MediaEngine.GetHeaderExtensionID().
//
// Note: pion/webrtc requires specifying allowed directions explicitly.
// For SendRecv, both Sendonly and Recvonly must be provided.
func RegisterHeaderExtensions(m *webrtc.MediaEngine, config HeaderExtensionConfig) error {
	// Validate configuration first
	if err := ValidateHeaderExtensionConfig(config); err != nil {
		return err
	}

	// Register video extensions
	for _, ext := range config.VideoExtensions {
		directions := toWebRTCDirections(ext.Direction)
		if err := m.RegisterHeaderExtension(
			webrtc.RTPHeaderExtensionCapability{URI: ext.URI},
			webrtc.RTPCodecTypeVideo,
			directions...,
		); err != nil {
			return err
		}
	}

	// Register audio extensions
	for _, ext := range config.AudioExtensions {
		directions := toWebRTCDirections(ext.Direction)
		if err := m.RegisterHeaderExtension(
			webrtc.RTPHeaderExtensionCapability{URI: ext.URI},
			webrtc.RTPCodecTypeAudio,
			directions...,
		); err != nil {
			return err
		}
	}

	return nil
}

// toWebRTCDirections converts ExtensionDirection to a slice of webrtc.RTPTransceiverDirection.
// pion/webrtc requires explicit direction specification:
// - For SendRecv: both Sendonly and Recvonly must be provided
// - For SendOnly: only Sendonly
// - For RecvOnly: only Recvonly
func toWebRTCDirections(d ExtensionDirection) []webrtc.RTPTransceiverDirection {
	switch d {
	case ExtensionDirectionSendOnly:
		return []webrtc.RTPTransceiverDirection{webrtc.RTPTransceiverDirectionSendonly}
	case ExtensionDirectionRecvOnly:
		return []webrtc.RTPTransceiverDirection{webrtc.RTPTransceiverDirectionRecvonly}
	default:
		// SendRecv requires both directions
		return []webrtc.RTPTransceiverDirection{
			webrtc.RTPTransceiverDirectionSendonly,
			webrtc.RTPTransceiverDirectionRecvonly,
		}
	}
}

// Copy creates a deep copy of the HeaderExtensionConfig.
func (c HeaderExtensionConfig) Copy() HeaderExtensionConfig {
	videoCopy := make([]HeaderExtension, len(c.VideoExtensions))
	copy(videoCopy, c.VideoExtensions)

	audioCopy := make([]HeaderExtension, len(c.AudioExtensions))
	copy(audioCopy, c.AudioExtensions)

	return HeaderExtensionConfig{
		VideoExtensions: videoCopy,
		AudioExtensions: audioCopy,
	}
}

// AddVideoExtension adds a video extension to the configuration.
func (c *HeaderExtensionConfig) AddVideoExtension(ext HeaderExtension) {
	c.VideoExtensions = append(c.VideoExtensions, ext)
}

// AddAudioExtension adds an audio extension to the configuration.
func (c *HeaderExtensionConfig) AddAudioExtension(ext HeaderExtension) {
	c.AudioExtensions = append(c.AudioExtensions, ext)
}

// RemoveVideoExtension removes a video extension by URI.
func (c *HeaderExtensionConfig) RemoveVideoExtension(uri string) {
	var filtered []HeaderExtension
	for _, ext := range c.VideoExtensions {
		if ext.URI != uri {
			filtered = append(filtered, ext)
		}
	}
	c.VideoExtensions = filtered
}

// RemoveAudioExtension removes an audio extension by URI.
func (c *HeaderExtensionConfig) RemoveAudioExtension(uri string) {
	var filtered []HeaderExtension
	for _, ext := range c.AudioExtensions {
		if ext.URI != uri {
			filtered = append(filtered, ext)
		}
	}
	c.AudioExtensions = filtered
}

// HasVideoExtension checks if a video extension with the given URI exists.
func (c HeaderExtensionConfig) HasVideoExtension(uri string) bool {
	for _, ext := range c.VideoExtensions {
		if ext.URI == uri {
			return true
		}
	}
	return false
}

// HasAudioExtension checks if an audio extension with the given URI exists.
func (c HeaderExtensionConfig) HasAudioExtension(uri string) bool {
	for _, ext := range c.AudioExtensions {
		if ext.URI == uri {
			return true
		}
	}
	return false
}

// GetVideoExtension returns the video extension with the given URI, or nil if not found.
func (c HeaderExtensionConfig) GetVideoExtension(uri string) *HeaderExtension {
	for i := range c.VideoExtensions {
		if c.VideoExtensions[i].URI == uri {
			return &c.VideoExtensions[i]
		}
	}
	return nil
}

// GetAudioExtension returns the audio extension with the given URI, or nil if not found.
func (c HeaderExtensionConfig) GetAudioExtension(uri string) *HeaderExtension {
	for i := range c.AudioExtensions {
		if c.AudioExtensions[i].URI == uri {
			return &c.AudioExtensions[i]
		}
	}
	return nil
}

// GetVideoExtensionURIs returns all video extension URIs.
func (c HeaderExtensionConfig) GetVideoExtensionURIs() []string {
	uris := make([]string, len(c.VideoExtensions))
	for i, ext := range c.VideoExtensions {
		uris[i] = ext.URI
	}
	return uris
}

// GetAudioExtensionURIs returns all audio extension URIs.
func (c HeaderExtensionConfig) GetAudioExtensionURIs() []string {
	uris := make([]string, len(c.AudioExtensions))
	for i, ext := range c.AudioExtensions {
		uris[i] = ext.URI
	}
	return uris
}

// ValidateHeaderExtensionConfig validates the header extension configuration.
// Returns an error if the configuration is invalid.
//
// Validation rules:
// - Extension IDs must be 1-14 (per RFC 8285, 0 and 15 are reserved for one-byte headers)
// - Extension IDs must be unique within each media type
// - Extension URIs must not be empty
// - Extension URIs must be unique within each media type
func ValidateHeaderExtensionConfig(config HeaderExtensionConfig) error {
	// Check for duplicate IDs and URIs in video extensions
	videoIDs := make(map[ExtensionID]bool)
	videoURIs := make(map[string]bool)
	for _, ext := range config.VideoExtensions {
		if videoIDs[ext.ID] {
			return ErrDuplicateExtensionID
		}
		// Per RFC 8285, one-byte header extension IDs are 1-14 (0 and 15 reserved)
		if ext.ID < 1 || ext.ID > 14 {
			return ErrInvalidExtensionID
		}
		if ext.URI == "" {
			return ErrEmptyExtensionURI
		}
		if videoURIs[ext.URI] {
			return ErrDuplicateExtensionURI
		}
		videoIDs[ext.ID] = true
		videoURIs[ext.URI] = true
	}

	// Check for duplicate IDs and URIs in audio extensions
	audioIDs := make(map[ExtensionID]bool)
	audioURIs := make(map[string]bool)
	for _, ext := range config.AudioExtensions {
		if audioIDs[ext.ID] {
			return ErrDuplicateExtensionID
		}
		// Per RFC 8285, one-byte header extension IDs are 1-14 (0 and 15 reserved)
		if ext.ID < 1 || ext.ID > 14 {
			return ErrInvalidExtensionID
		}
		if ext.URI == "" {
			return ErrEmptyExtensionURI
		}
		if audioURIs[ext.URI] {
			return ErrDuplicateExtensionURI
		}
		audioIDs[ext.ID] = true
		audioURIs[ext.URI] = true
	}

	return nil
}

// RequiredVideoExtensions returns the list of required video extension URIs per spec.
func RequiredVideoExtensions() []string {
	return []string{
		ExtensionURIMID,
		ExtensionURIRID,
		ExtensionURITransportCC,
		ExtensionURIAbsSendTime,
	}
}

// RequiredAudioExtensions returns the list of required audio extension URIs per spec.
func RequiredAudioExtensions() []string {
	return []string{
		ExtensionURIMID,
		ExtensionURITransportCC,
		ExtensionURIAbsSendTime,
	}
}

// HasRequiredVideoExtensions checks if all required video extensions are present.
func (c HeaderExtensionConfig) HasRequiredVideoExtensions() bool {
	for _, uri := range RequiredVideoExtensions() {
		if !c.HasVideoExtension(uri) {
			return false
		}
	}
	return true
}

// HasRequiredAudioExtensions checks if all required audio extensions are present.
func (c HeaderExtensionConfig) HasRequiredAudioExtensions() bool {
	for _, uri := range RequiredAudioExtensions() {
		if !c.HasAudioExtension(uri) {
			return false
		}
	}
	return true
}

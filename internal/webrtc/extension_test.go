package webrtc

import (
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestDefaultHeaderExtensionConfig(t *testing.T) {
	config := DefaultHeaderExtensionConfig()

	// Verify video extensions
	expectedVideoURIs := []string{
		ExtensionURIMID,
		ExtensionURIRID,
		ExtensionURIRepairedRID,
		ExtensionURIAbsSendTime,
		ExtensionURITransportCC,
		ExtensionURIVideoOrientation,
	}

	if len(config.VideoExtensions) != len(expectedVideoURIs) {
		t.Errorf("expected %d video extensions, got %d", len(expectedVideoURIs), len(config.VideoExtensions))
	}

	for _, uri := range expectedVideoURIs {
		if !config.HasVideoExtension(uri) {
			t.Errorf("expected video extension %s to be present", uri)
		}
	}

	// Verify audio extensions
	expectedAudioURIs := []string{
		ExtensionURIMID,
		ExtensionURIAbsSendTime,
		ExtensionURITransportCC,
		ExtensionURIAudioLevel,
	}

	if len(config.AudioExtensions) != len(expectedAudioURIs) {
		t.Errorf("expected %d audio extensions, got %d", len(expectedAudioURIs), len(config.AudioExtensions))
	}

	for _, uri := range expectedAudioURIs {
		if !config.HasAudioExtension(uri) {
			t.Errorf("expected audio extension %s to be present", uri)
		}
	}
}

func TestMinimalHeaderExtensionConfig(t *testing.T) {
	config := MinimalHeaderExtensionConfig()

	// Verify required video extensions are present
	requiredVideo := RequiredVideoExtensions()
	for _, uri := range requiredVideo {
		if !config.HasVideoExtension(uri) {
			t.Errorf("minimal config should have required video extension %s", uri)
		}
	}

	// Verify required audio extensions are present
	requiredAudio := RequiredAudioExtensions()
	for _, uri := range requiredAudio {
		if !config.HasAudioExtension(uri) {
			t.Errorf("minimal config should have required audio extension %s", uri)
		}
	}
}

func TestRegisterHeaderExtensions(t *testing.T) {
	tests := []struct {
		name    string
		config  HeaderExtensionConfig
		wantErr bool
	}{
		{
			name:    "default config",
			config:  DefaultHeaderExtensionConfig(),
			wantErr: false,
		},
		{
			name:    "minimal config",
			config:  MinimalHeaderExtensionConfig(),
			wantErr: false,
		},
		{
			name: "single video extension",
			config: HeaderExtensionConfig{
				VideoExtensions: []HeaderExtension{
					{ID: 1, URI: ExtensionURIMID, Direction: ExtensionDirectionSendRecv},
				},
				AudioExtensions: []HeaderExtension{},
			},
			wantErr: false,
		},
		{
			name: "single audio extension",
			config: HeaderExtensionConfig{
				VideoExtensions: []HeaderExtension{},
				AudioExtensions: []HeaderExtension{
					{ID: 1, URI: ExtensionURIMID, Direction: ExtensionDirectionSendRecv},
				},
			},
			wantErr: false,
		},
		{
			name: "empty config",
			config: HeaderExtensionConfig{
				VideoExtensions: []HeaderExtension{},
				AudioExtensions: []HeaderExtension{},
			},
			wantErr: false,
		},
		{
			name: "invalid config - duplicate video extension ID",
			config: HeaderExtensionConfig{
				VideoExtensions: []HeaderExtension{
					{ID: 1, URI: ExtensionURIMID, Direction: ExtensionDirectionSendRecv},
					{ID: 1, URI: ExtensionURIRID, Direction: ExtensionDirectionSendRecv},
				},
				AudioExtensions: []HeaderExtension{},
			},
			wantErr: true,
		},
		{
			name: "invalid config - invalid extension ID",
			config: HeaderExtensionConfig{
				VideoExtensions: []HeaderExtension{
					{ID: 0, URI: ExtensionURIMID, Direction: ExtensionDirectionSendRecv},
				},
				AudioExtensions: []HeaderExtension{},
			},
			wantErr: true,
		},
		{
			name: "invalid config - empty URI",
			config: HeaderExtensionConfig{
				VideoExtensions: []HeaderExtension{
					{ID: 1, URI: "", Direction: ExtensionDirectionSendRecv},
				},
				AudioExtensions: []HeaderExtension{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &webrtc.MediaEngine{}
			err := RegisterHeaderExtensions(m, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterHeaderExtensions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateHeaderExtensionConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  HeaderExtensionConfig
		wantErr error
	}{
		{
			name:    "valid default config",
			config:  DefaultHeaderExtensionConfig(),
			wantErr: nil,
		},
		{
			name:    "valid minimal config",
			config:  MinimalHeaderExtensionConfig(),
			wantErr: nil,
		},
		{
			name: "duplicate video extension ID",
			config: HeaderExtensionConfig{
				VideoExtensions: []HeaderExtension{
					{ID: 1, URI: ExtensionURIMID, Direction: ExtensionDirectionSendRecv},
					{ID: 1, URI: ExtensionURIRID, Direction: ExtensionDirectionSendRecv},
				},
				AudioExtensions: []HeaderExtension{},
			},
			wantErr: ErrDuplicateExtensionID,
		},
		{
			name: "duplicate audio extension ID",
			config: HeaderExtensionConfig{
				VideoExtensions: []HeaderExtension{},
				AudioExtensions: []HeaderExtension{
					{ID: 1, URI: ExtensionURIMID, Direction: ExtensionDirectionSendRecv},
					{ID: 1, URI: ExtensionURITransportCC, Direction: ExtensionDirectionSendRecv},
				},
			},
			wantErr: ErrDuplicateExtensionID,
		},
		{
			name: "invalid extension ID (zero)",
			config: HeaderExtensionConfig{
				VideoExtensions: []HeaderExtension{
					{ID: 0, URI: ExtensionURIMID, Direction: ExtensionDirectionSendRecv},
				},
				AudioExtensions: []HeaderExtension{},
			},
			wantErr: ErrInvalidExtensionID,
		},
		{
			name: "invalid extension ID (> 15)",
			config: HeaderExtensionConfig{
				VideoExtensions: []HeaderExtension{
					{ID: 16, URI: ExtensionURIMID, Direction: ExtensionDirectionSendRecv},
				},
				AudioExtensions: []HeaderExtension{},
			},
			wantErr: ErrInvalidExtensionID,
		},
		{
			name: "empty extension URI",
			config: HeaderExtensionConfig{
				VideoExtensions: []HeaderExtension{
					{ID: 1, URI: "", Direction: ExtensionDirectionSendRecv},
				},
				AudioExtensions: []HeaderExtension{},
			},
			wantErr: ErrEmptyExtensionURI,
		},
		{
			name: "invalid extension ID (15 - reserved per RFC 8285)",
			config: HeaderExtensionConfig{
				VideoExtensions: []HeaderExtension{
					{ID: 15, URI: ExtensionURIMID, Direction: ExtensionDirectionSendRecv},
				},
				AudioExtensions: []HeaderExtension{},
			},
			wantErr: ErrInvalidExtensionID,
		},
		{
			name: "valid edge case ID 14",
			config: HeaderExtensionConfig{
				VideoExtensions: []HeaderExtension{
					{ID: 14, URI: ExtensionURIMID, Direction: ExtensionDirectionSendRecv},
				},
				AudioExtensions: []HeaderExtension{},
			},
			wantErr: nil,
		},
		{
			name: "duplicate video extension URI",
			config: HeaderExtensionConfig{
				VideoExtensions: []HeaderExtension{
					{ID: 1, URI: ExtensionURIMID, Direction: ExtensionDirectionSendRecv},
					{ID: 2, URI: ExtensionURIMID, Direction: ExtensionDirectionSendRecv},
				},
				AudioExtensions: []HeaderExtension{},
			},
			wantErr: ErrDuplicateExtensionURI,
		},
		{
			name: "duplicate audio extension URI",
			config: HeaderExtensionConfig{
				VideoExtensions: []HeaderExtension{},
				AudioExtensions: []HeaderExtension{
					{ID: 1, URI: ExtensionURIMID, Direction: ExtensionDirectionSendRecv},
					{ID: 2, URI: ExtensionURIMID, Direction: ExtensionDirectionSendRecv},
				},
			},
			wantErr: ErrDuplicateExtensionURI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHeaderExtensionConfig(tt.config)
			if err != tt.wantErr {
				t.Errorf("ValidateHeaderExtensionConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHeaderExtensionConfigCopy(t *testing.T) {
	original := DefaultHeaderExtensionConfig()
	copied := original.Copy()

	// Verify lengths match
	if len(copied.VideoExtensions) != len(original.VideoExtensions) {
		t.Errorf("copy video extensions length mismatch")
	}
	if len(copied.AudioExtensions) != len(original.AudioExtensions) {
		t.Errorf("copy audio extensions length mismatch")
	}

	// Modify copy and verify original is unchanged
	if len(copied.VideoExtensions) > 0 {
		copied.VideoExtensions[0].URI = "modified"
		if original.VideoExtensions[0].URI == "modified" {
			t.Errorf("modifying copy should not affect original")
		}
	}
}

func TestAddRemoveExtensions(t *testing.T) {
	config := HeaderExtensionConfig{
		VideoExtensions: []HeaderExtension{},
		AudioExtensions: []HeaderExtension{},
	}

	// Test AddVideoExtension
	config.AddVideoExtension(HeaderExtension{
		ID:        1,
		URI:       ExtensionURIMID,
		Direction: ExtensionDirectionSendRecv,
	})
	if !config.HasVideoExtension(ExtensionURIMID) {
		t.Error("AddVideoExtension failed")
	}

	// Test AddAudioExtension
	config.AddAudioExtension(HeaderExtension{
		ID:        1,
		URI:       ExtensionURIAudioLevel,
		Direction: ExtensionDirectionSendRecv,
	})
	if !config.HasAudioExtension(ExtensionURIAudioLevel) {
		t.Error("AddAudioExtension failed")
	}

	// Test RemoveVideoExtension
	config.RemoveVideoExtension(ExtensionURIMID)
	if config.HasVideoExtension(ExtensionURIMID) {
		t.Error("RemoveVideoExtension failed")
	}

	// Test RemoveAudioExtension
	config.RemoveAudioExtension(ExtensionURIAudioLevel)
	if config.HasAudioExtension(ExtensionURIAudioLevel) {
		t.Error("RemoveAudioExtension failed")
	}
}

func TestGetExtensions(t *testing.T) {
	config := DefaultHeaderExtensionConfig()

	// Test GetVideoExtension
	ext := config.GetVideoExtension(ExtensionURIMID)
	if ext == nil {
		t.Error("GetVideoExtension returned nil for existing extension")
	}
	if ext.URI != ExtensionURIMID {
		t.Error("GetVideoExtension returned wrong extension")
	}

	// Test GetVideoExtension for non-existent
	ext = config.GetVideoExtension("non-existent")
	if ext != nil {
		t.Error("GetVideoExtension should return nil for non-existent extension")
	}

	// Test GetAudioExtension
	ext = config.GetAudioExtension(ExtensionURIMID)
	if ext == nil {
		t.Error("GetAudioExtension returned nil for existing extension")
	}

	// Test GetAudioExtension for non-existent
	ext = config.GetAudioExtension("non-existent")
	if ext != nil {
		t.Error("GetAudioExtension should return nil for non-existent extension")
	}
}

func TestGetExtensionURIs(t *testing.T) {
	config := MinimalHeaderExtensionConfig()

	videoURIs := config.GetVideoExtensionURIs()
	if len(videoURIs) != len(config.VideoExtensions) {
		t.Errorf("GetVideoExtensionURIs returned wrong number of URIs")
	}

	audioURIs := config.GetAudioExtensionURIs()
	if len(audioURIs) != len(config.AudioExtensions) {
		t.Errorf("GetAudioExtensionURIs returned wrong number of URIs")
	}
}

func TestHasRequiredExtensions(t *testing.T) {
	// Default config should have all required extensions
	defaultConfig := DefaultHeaderExtensionConfig()
	if !defaultConfig.HasRequiredVideoExtensions() {
		t.Error("default config should have all required video extensions")
	}
	if !defaultConfig.HasRequiredAudioExtensions() {
		t.Error("default config should have all required audio extensions")
	}

	// Minimal config should also have all required extensions
	minimalConfig := MinimalHeaderExtensionConfig()
	if !minimalConfig.HasRequiredVideoExtensions() {
		t.Error("minimal config should have all required video extensions")
	}
	if !minimalConfig.HasRequiredAudioExtensions() {
		t.Error("minimal config should have all required audio extensions")
	}

	// Empty config should not have required extensions
	emptyConfig := HeaderExtensionConfig{}
	if emptyConfig.HasRequiredVideoExtensions() {
		t.Error("empty config should not have required video extensions")
	}
	if emptyConfig.HasRequiredAudioExtensions() {
		t.Error("empty config should not have required audio extensions")
	}
}

func TestToWebRTCDirections(t *testing.T) {
	tests := []struct {
		input    ExtensionDirection
		expected []webrtc.RTPTransceiverDirection
	}{
		{
			ExtensionDirectionSendRecv,
			[]webrtc.RTPTransceiverDirection{
				webrtc.RTPTransceiverDirectionSendonly,
				webrtc.RTPTransceiverDirectionRecvonly,
			},
		},
		{
			ExtensionDirectionSendOnly,
			[]webrtc.RTPTransceiverDirection{webrtc.RTPTransceiverDirectionSendonly},
		},
		{
			ExtensionDirectionRecvOnly,
			[]webrtc.RTPTransceiverDirection{webrtc.RTPTransceiverDirectionRecvonly},
		},
	}

	for _, tt := range tests {
		result := toWebRTCDirections(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("toWebRTCDirections(%d) returned %d directions, want %d", tt.input, len(result), len(tt.expected))
			continue
		}
		for i, dir := range result {
			if dir != tt.expected[i] {
				t.Errorf("toWebRTCDirections(%d)[%d] = %v, want %v", tt.input, i, dir, tt.expected[i])
			}
		}
	}
}

func TestExtensionURIConstants(t *testing.T) {
	// Verify URI constants are not empty
	uris := []string{
		ExtensionURIMID,
		ExtensionURIRID,
		ExtensionURIRepairedRID,
		ExtensionURITransportCC,
		ExtensionURIAbsSendTime,
		ExtensionURIAudioLevel,
		ExtensionURIVideoOrientation,
	}

	for _, uri := range uris {
		if uri == "" {
			t.Errorf("extension URI constant should not be empty")
		}
	}

	// Verify specific URI values per spec
	if ExtensionURIMID != "urn:ietf:params:rtp-hdrext:sdes:mid" {
		t.Errorf("ExtensionURIMID has incorrect value")
	}
	if ExtensionURIRID != "urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id" {
		t.Errorf("ExtensionURIRID has incorrect value")
	}
	if ExtensionURITransportCC != "http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01" {
		t.Errorf("ExtensionURITransportCC has incorrect value")
	}
	if ExtensionURIAbsSendTime != "http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time" {
		t.Errorf("ExtensionURIAbsSendTime has incorrect value")
	}
}

func TestRegisterHeaderExtensionsWithCodecs(t *testing.T) {
	// Test that extensions can be registered alongside codecs
	m := &webrtc.MediaEngine{}

	// First register codecs
	if err := RegisterCodecs(m, DefaultCodecConfig()); err != nil {
		t.Fatalf("failed to register codecs: %v", err)
	}

	// Then register header extensions
	if err := RegisterHeaderExtensions(m, DefaultHeaderExtensionConfig()); err != nil {
		t.Fatalf("failed to register header extensions: %v", err)
	}
}

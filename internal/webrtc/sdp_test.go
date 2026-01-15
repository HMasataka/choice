package webrtc

import (
	"errors"
	"testing"

	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
)

// Sample SDP for testing (Unified Plan format)
const testSDPOffer = `v=0
o=- 4596489990601351948 2 IN IP4 127.0.0.1
s=-
t=0 0
a=group:BUNDLE 0 1
a=extmap-allow-mixed
a=msid-semantic: WMS
m=audio 9 UDP/TLS/RTP/SAVPF 111 63 9 0 8 13 110 126
c=IN IP4 0.0.0.0
a=rtcp:9 IN IP4 0.0.0.0
a=ice-ufrag:test
a=ice-pwd:testpassword123456789012
a=ice-options:trickle
a=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00
a=setup:actpass
a=mid:0
a=extmap:1 urn:ietf:params:rtp-hdrext:ssrc-audio-level
a=extmap:2 http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time
a=sendrecv
a=rtcp-mux
a=rtpmap:111 opus/48000/2
a=rtcp-fb:111 transport-cc
a=fmtp:111 minptime=10;useinbandfec=1
a=rtpmap:63 red/48000/2
a=fmtp:63 111/111
a=rtpmap:9 G722/8000
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:13 CN/8000
a=rtpmap:110 telephone-event/48000
a=rtpmap:126 telephone-event/8000
a=ssrc:1234567890 cname:test
m=video 9 UDP/TLS/RTP/SAVPF 96 97 98 99 100
c=IN IP4 0.0.0.0
a=rtcp:9 IN IP4 0.0.0.0
a=ice-ufrag:test
a=ice-pwd:testpassword123456789012
a=ice-options:trickle
a=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00
a=setup:actpass
a=mid:1
a=extmap:14 urn:ietf:params:rtp-hdrext:toffset
a=extmap:2 http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time
a=extmap:13 urn:3gpp:video-orientation
a=extmap:3 http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01
a=sendrecv
a=rtcp-mux
a=rtcp-rsize
a=rtpmap:96 VP8/90000
a=rtcp-fb:96 goog-remb
a=rtcp-fb:96 transport-cc
a=rtcp-fb:96 ccm fir
a=rtcp-fb:96 nack
a=rtcp-fb:96 nack pli
a=rtpmap:97 rtx/90000
a=fmtp:97 apt=96
a=rtpmap:98 VP9/90000
a=rtcp-fb:98 goog-remb
a=rtcp-fb:98 transport-cc
a=rtcp-fb:98 ccm fir
a=rtcp-fb:98 nack
a=rtcp-fb:98 nack pli
a=fmtp:98 profile-id=0
a=rtpmap:99 rtx/90000
a=fmtp:99 apt=98
a=rtpmap:100 H264/90000
a=rtcp-fb:100 goog-remb
a=rtcp-fb:100 transport-cc
a=rtcp-fb:100 ccm fir
a=rtcp-fb:100 nack
a=rtcp-fb:100 nack pli
a=fmtp:100 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f
a=ssrc:987654321 cname:test
`

const testSDPWithoutBundle = `v=0
o=- 4596489990601351948 2 IN IP4 127.0.0.1
s=-
t=0 0
m=audio 9 UDP/TLS/RTP/SAVPF 111
c=IN IP4 0.0.0.0
a=rtcp-mux
a=mid:0
a=rtpmap:111 opus/48000/2
`

const testSDPWithoutRTCPMux = `v=0
o=- 4596489990601351948 2 IN IP4 127.0.0.1
s=-
t=0 0
a=group:BUNDLE 0
m=audio 9 UDP/TLS/RTP/SAVPF 111
c=IN IP4 0.0.0.0
a=mid:0
a=rtpmap:111 opus/48000/2
`

const testSDPMissingMID = `v=0
o=- 4596489990601351948 2 IN IP4 127.0.0.1
s=-
t=0 0
a=group:BUNDLE 0
m=audio 9 UDP/TLS/RTP/SAVPF 111
c=IN IP4 0.0.0.0
a=rtcp-mux
a=rtpmap:111 opus/48000/2
`

const testSDPDuplicateMID = `v=0
o=- 4596489990601351948 2 IN IP4 127.0.0.1
s=-
t=0 0
a=group:BUNDLE 0 0
m=audio 9 UDP/TLS/RTP/SAVPF 111
c=IN IP4 0.0.0.0
a=rtcp-mux
a=mid:0
a=rtpmap:111 opus/48000/2
m=video 9 UDP/TLS/RTP/SAVPF 96
c=IN IP4 0.0.0.0
a=rtcp-mux
a=mid:0
a=rtpmap:96 VP8/90000
`

const testSDPWithSimulcast = `v=0
o=- 4596489990601351948 2 IN IP4 127.0.0.1
s=-
t=0 0
a=group:BUNDLE 0
m=video 9 UDP/TLS/RTP/SAVPF 96
c=IN IP4 0.0.0.0
a=rtcp-mux
a=mid:0
a=rid:h send
a=rid:m send
a=rid:l send
a=simulcast:send h;m;l
a=rtpmap:96 VP8/90000
`

func TestNewSDPProcessor(t *testing.T) {
	config := DefaultSDPConfig()
	processor := NewSDPProcessor(config)

	if processor == nil {
		t.Error("expected non-nil processor")
	}

	if !processor.config.EnableSafariCompat {
		t.Error("expected EnableSafariCompat to be true by default")
	}
}

func TestDefaultSDPConfig(t *testing.T) {
	config := DefaultSDPConfig()

	if !config.EnableSafariCompat {
		t.Error("expected EnableSafariCompat to be true by default")
	}
}

func TestSDPProcessor_ParseSDP(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	tests := []struct {
		name    string
		sdp     string
		wantErr bool
	}{
		{
			name:    "valid SDP",
			sdp:     testSDPOffer,
			wantErr: false,
		},
		{
			name:    "empty SDP",
			sdp:     "",
			wantErr: true,
		},
		{
			name:    "invalid SDP",
			sdp:     "invalid sdp content",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sd, err := processor.ParseSDP(tt.sdp)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if sd == nil {
					t.Error("expected non-nil SessionDescription")
				}
			}
		})
	}
}

func TestSDPProcessor_ValidateOffer(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	tests := []struct {
		name    string
		offer   webrtc.SessionDescription
		wantErr error
	}{
		{
			name: "valid offer with BUNDLE and rtcp-mux",
			offer: webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  testSDPOffer,
			},
			wantErr: nil,
		},
		{
			name: "invalid type (answer instead of offer)",
			offer: webrtc.SessionDescription{
				Type: webrtc.SDPTypeAnswer,
				SDP:  testSDPOffer,
			},
			wantErr: ErrInvalidSDPFormat,
		},
		{
			name: "missing BUNDLE",
			offer: webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  testSDPWithoutBundle,
			},
			wantErr: ErrMissingBundle,
		},
		{
			name: "missing rtcp-mux",
			offer: webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  testSDPWithoutRTCPMux,
			},
			wantErr: ErrMissingRTCPMux,
		},
		{
			name: "missing MID (Plan B indicator)",
			offer: webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  testSDPMissingMID,
			},
			wantErr: ErrUnsupportedSDPSemantics,
		},
		{
			name: "duplicate MID",
			offer: webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  testSDPDuplicateMID,
			},
			wantErr: ErrUnsupportedSDPSemantics,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := processor.ValidateOffer(tt.offer)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestSDPProcessor_ValidateAnswer(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	tests := []struct {
		name    string
		answer  webrtc.SessionDescription
		wantErr error
	}{
		{
			name: "valid answer",
			answer: webrtc.SessionDescription{
				Type: webrtc.SDPTypeAnswer,
				SDP:  testSDPOffer,
			},
			wantErr: nil,
		},
		{
			name: "invalid type",
			answer: webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  testSDPOffer,
			},
			wantErr: ErrInvalidSDPFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := processor.ValidateAnswer(tt.answer)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestSDPProcessor_ExtractMIDs(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	mids, err := processor.ExtractMIDs(testSDPOffer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mids) != 2 {
		t.Errorf("expected 2 MIDs, got %d", len(mids))
	}

	// Check expected MIDs
	expectedMIDs := map[string]bool{"0": true, "1": true}
	for _, mid := range mids {
		if !expectedMIDs[mid] {
			t.Errorf("unexpected MID: %s", mid)
		}
	}
}

func TestSDPProcessor_ExtractCodecs(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	codecs, err := processor.ExtractCodecs(testSDPOffer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(codecs) == 0 {
		t.Error("expected codecs to be extracted")
	}

	// Check for expected codecs
	foundOpus := false
	foundVP8 := false
	foundH264 := false

	for _, codec := range codecs {
		switch codec.Name {
		case "opus":
			foundOpus = true
		case "VP8":
			foundVP8 = true
		case "H264":
			foundH264 = true
		}
	}

	if !foundOpus {
		t.Error("expected to find Opus codec")
	}
	if !foundVP8 {
		t.Error("expected to find VP8 codec")
	}
	if !foundH264 {
		t.Error("expected to find H264 codec")
	}
}

func TestSDPProcessor_GetBundleGroup(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	bundle, err := processor.GetBundleGroup(testSDPOffer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(bundle) != 2 {
		t.Errorf("expected 2 bundle members, got %d", len(bundle))
	}
}

func TestSDPProcessor_GetBundleGroup_Missing(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	_, err := processor.GetBundleGroup(testSDPWithoutBundle)
	if !errors.Is(err, ErrMissingBundle) {
		t.Errorf("expected ErrMissingBundle, got %v", err)
	}
}

func TestSDPProcessor_IsSimulcastEnabled(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	tests := []struct {
		name     string
		sdp      string
		expected bool
	}{
		{
			name:     "simulcast enabled",
			sdp:      testSDPWithSimulcast,
			expected: true,
		},
		{
			name:     "simulcast disabled",
			sdp:      testSDPOffer,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled, err := processor.IsSimulcastEnabled(tt.sdp)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if enabled != tt.expected {
				t.Errorf("expected simulcast enabled=%v, got %v", tt.expected, enabled)
			}
		})
	}
}

func TestSDPProcessor_ExtractSimulcastLayers(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	layers, err := processor.ExtractSimulcastLayers(testSDPWithSimulcast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(layers) != 3 {
		t.Errorf("expected 3 simulcast layers, got %d", len(layers))
	}

	// Check layer RIDs
	rids := make(map[string]bool)
	for _, layer := range layers {
		rids[layer.RID] = true
	}

	expectedRIDs := []string{"h", "m", "l"}
	for _, rid := range expectedRIDs {
		if !rids[rid] {
			t.Errorf("expected RID %s not found", rid)
		}
	}
}

func TestSDPProcessor_NormalizeForSafari(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	normalized, err := processor.NormalizeForSafari(testSDPOffer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if normalized == "" {
		t.Error("expected non-empty normalized SDP")
	}
}

func TestSDPProcessor_NormalizeForSafari_Disabled(t *testing.T) {
	config := SDPConfig{EnableSafariCompat: false}
	processor := NewSDPProcessor(config)

	original := testSDPOffer
	normalized, err := processor.NormalizeForSafari(original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if normalized != original {
		t.Error("expected SDP to be unchanged when Safari compat is disabled")
	}
}

func TestSDPProcessor_GetICECredentials(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	ufrag, pwd, err := processor.GetICECredentials(testSDPOffer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ufrag != "test" {
		t.Errorf("expected ufrag 'test', got '%s'", ufrag)
	}

	if pwd != "testpassword123456789012" {
		t.Errorf("expected pwd 'testpassword123456789012', got '%s'", pwd)
	}
}

func TestSDPProcessor_GetFingerprint(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	algo, fp, err := processor.GetFingerprint(testSDPOffer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if algo != "sha-256" {
		t.Errorf("expected algorithm 'sha-256', got '%s'", algo)
	}

	if fp == "" {
		t.Error("expected non-empty fingerprint")
	}
}

func TestSDPProcessor_ExtractSSRCs(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	ssrcs, err := processor.ExtractSSRCs(testSDPOffer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ssrcs) < 2 {
		t.Errorf("expected at least 2 SSRCs, got %d", len(ssrcs))
	}

	// Check for expected SSRCs
	ssrcMap := make(map[uint32]bool)
	for _, ssrc := range ssrcs {
		ssrcMap[ssrc] = true
	}

	if !ssrcMap[1234567890] {
		t.Error("expected SSRC 1234567890 not found")
	}
	if !ssrcMap[987654321] {
		t.Error("expected SSRC 987654321 not found")
	}
}

func TestSDPProcessor_SetCodecPriority(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	// Set VP9 as highest priority for video
	modified, err := processor.SetCodecPriority(testSDPOffer, "video", []string{"VP9", "VP8", "H264"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the modified SDP is valid
	_, err = processor.ParseSDP(modified)
	if err != nil {
		t.Fatalf("modified SDP is invalid: %v", err)
	}
}

func TestSDPProcessor_RemoveCodec(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	// Remove VP9 codec
	modified, err := processor.RemoveCodec(testSDPOffer, "VP9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify VP9 is removed
	codecs, err := processor.ExtractCodecs(modified)
	if err != nil {
		t.Fatalf("unexpected error extracting codecs: %v", err)
	}

	for _, codec := range codecs {
		if codec.Name == "VP9" {
			t.Error("VP9 should have been removed")
		}
	}
}

func TestSDPProcessor_AddAttribute(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	modified, err := processor.AddAttribute(testSDPOffer, "test-attr", "test-value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the attribute was added
	sd, err := processor.ParseSDP(modified)
	if err != nil {
		t.Fatalf("modified SDP is invalid: %v", err)
	}

	found := false
	for _, attr := range sd.Attributes {
		if attr.Key == "test-attr" && attr.Value == "test-value" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected attribute not found in modified SDP")
	}
}

func TestSDPProcessor_ModifySDP(t *testing.T) {
	processor := NewSDPProcessor(DefaultSDPConfig())

	// Modify the session name
	modified, err := processor.ModifySDP(testSDPOffer, func(sd *sdp.SessionDescription) error {
		sd.SessionName = "Modified Session"
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sd, err := processor.ParseSDP(modified)
	if err != nil {
		t.Fatalf("modified SDP is invalid: %v", err)
	}

	if sd.SessionName != "Modified Session" {
		t.Errorf("expected session name 'Modified Session', got '%s'", sd.SessionName)
	}
}

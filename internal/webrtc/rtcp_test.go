package webrtc

import (
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestDefaultVideoRTCPFeedback(t *testing.T) {
	feedback := DefaultVideoRTCPFeedback()

	// Should have 5 feedback entries for video
	if len(feedback) != 5 {
		t.Errorf("expected 5 video feedback entries, got %d", len(feedback))
	}

	// Verify all required entries are present
	required := []RTCPFeedback{
		FeedbackNACK,
		FeedbackNACKPLI,
		FeedbackCCMFIR,
		FeedbackGoogREMB,
		FeedbackTransportCC,
	}

	for _, r := range required {
		if !HasFeedback(feedback, r) {
			t.Errorf("missing required video feedback: %s", r.SDPFeedbackString())
		}
	}
}

func TestDefaultAudioRTCPFeedback(t *testing.T) {
	feedback := DefaultAudioRTCPFeedback()

	// Should have 1 feedback entry for audio
	if len(feedback) != 1 {
		t.Errorf("expected 1 audio feedback entry, got %d", len(feedback))
	}

	// Verify transport-cc is present
	if !HasFeedback(feedback, FeedbackTransportCC) {
		t.Error("missing required audio feedback: transport-cc")
	}
}

func TestDefaultRTCPFeedbackConfig(t *testing.T) {
	config := DefaultRTCPFeedbackConfig()

	if len(config.VideoFeedback) != 5 {
		t.Errorf("expected 5 video feedback entries, got %d", len(config.VideoFeedback))
	}

	if len(config.AudioFeedback) != 1 {
		t.Errorf("expected 1 audio feedback entry, got %d", len(config.AudioFeedback))
	}
}

func TestRTCPFeedback_ToWebRTCFeedback(t *testing.T) {
	tests := []struct {
		name     string
		feedback RTCPFeedback
		expected webrtc.RTCPFeedback
	}{
		{
			name:     "nack",
			feedback: FeedbackNACK,
			expected: webrtc.RTCPFeedback{Type: "nack", Parameter: ""},
		},
		{
			name:     "nack pli",
			feedback: FeedbackNACKPLI,
			expected: webrtc.RTCPFeedback{Type: "nack", Parameter: "pli"},
		},
		{
			name:     "ccm fir",
			feedback: FeedbackCCMFIR,
			expected: webrtc.RTCPFeedback{Type: "ccm", Parameter: "fir"},
		},
		{
			name:     "goog-remb",
			feedback: FeedbackGoogREMB,
			expected: webrtc.RTCPFeedback{Type: "goog-remb", Parameter: ""},
		},
		{
			name:     "transport-cc",
			feedback: FeedbackTransportCC,
			expected: webrtc.RTCPFeedback{Type: "transport-cc", Parameter: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.feedback.ToWebRTCFeedback()
			if result.Type != tt.expected.Type {
				t.Errorf("expected Type %q, got %q", tt.expected.Type, result.Type)
			}
			if result.Parameter != tt.expected.Parameter {
				t.Errorf("expected Parameter %q, got %q", tt.expected.Parameter, result.Parameter)
			}
		})
	}
}

func TestToWebRTCFeedbackSlice(t *testing.T) {
	feedback := []RTCPFeedback{FeedbackNACK, FeedbackNACKPLI}

	result := ToWebRTCFeedbackSlice(feedback)

	if len(result) != 2 {
		t.Errorf("expected 2 feedback items, got %d", len(result))
	}

	// Verify first item (nack)
	if result[0].Type != "nack" || result[0].Parameter != "" {
		t.Errorf("expected first: {nack, \"\"}, got {%s, %s}", result[0].Type, result[0].Parameter)
	}

	// Verify second item (nack pli)
	if result[1].Type != "nack" || result[1].Parameter != "pli" {
		t.Errorf("expected second: {nack, pli}, got {%s, %s}", result[1].Type, result[1].Parameter)
	}
}

func TestToWebRTCFeedbackSlice_Empty(t *testing.T) {
	feedback := []RTCPFeedback{}
	result := ToWebRTCFeedbackSlice(feedback)

	if len(result) != 0 {
		t.Errorf("expected 0 feedback items, got %d", len(result))
	}
}

func TestRTCPFeedbackFromWebRTC(t *testing.T) {
	tests := []struct {
		name     string
		input    webrtc.RTCPFeedback
		expected RTCPFeedback
	}{
		{
			name:     "nack",
			input:    webrtc.RTCPFeedback{Type: "nack", Parameter: ""},
			expected: FeedbackNACK,
		},
		{
			name:     "nack pli",
			input:    webrtc.RTCPFeedback{Type: "nack", Parameter: "pli"},
			expected: FeedbackNACKPLI,
		},
		{
			name:     "ccm fir",
			input:    webrtc.RTCPFeedback{Type: "ccm", Parameter: "fir"},
			expected: FeedbackCCMFIR,
		},
		{
			name:     "transport-cc",
			input:    webrtc.RTCPFeedback{Type: "transport-cc", Parameter: ""},
			expected: FeedbackTransportCC,
		},
		{
			name:     "goog-remb",
			input:    webrtc.RTCPFeedback{Type: "goog-remb", Parameter: ""},
			expected: FeedbackGoogREMB,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RTCPFeedbackFromWebRTC(tt.input)
			if !result.Equals(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestRTCPFeedback_Equals(t *testing.T) {
	tests := []struct {
		name     string
		a        RTCPFeedback
		b        RTCPFeedback
		expected bool
	}{
		{
			name:     "same type and param",
			a:        FeedbackNACKPLI,
			b:        RTCPFeedback{Type: RTCPFeedbackTypeNACK, Parameter: RTCPFeedbackParamPLI},
			expected: true,
		},
		{
			name:     "same type different param",
			a:        FeedbackNACK,
			b:        FeedbackNACKPLI,
			expected: false,
		},
		{
			name:     "different type same param",
			a:        FeedbackGoogREMB,
			b:        FeedbackTransportCC,
			expected: false,
		},
		{
			name:     "both empty param",
			a:        FeedbackTransportCC,
			b:        RTCPFeedback{Type: RTCPFeedbackTypeTransportCC, Parameter: RTCPFeedbackParamNone},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.a.Equals(tt.b)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestHasFeedback(t *testing.T) {
	feedback := []RTCPFeedback{FeedbackNACK, FeedbackNACKPLI, FeedbackTransportCC}

	tests := []struct {
		name     string
		target   RTCPFeedback
		expected bool
	}{
		{"has nack", FeedbackNACK, true},
		{"has nack pli", FeedbackNACKPLI, true},
		{"has transport-cc", FeedbackTransportCC, true},
		{"missing ccm fir", FeedbackCCMFIR, false},
		{"missing goog-remb", FeedbackGoogREMB, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasFeedback(feedback, tt.target)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestHasFeedback_DistinguishesNackTypes(t *testing.T) {
	// Only has nack, not nack pli
	feedback := []RTCPFeedback{FeedbackNACK}

	if !HasFeedback(feedback, FeedbackNACK) {
		t.Error("expected to have nack")
	}

	if HasFeedback(feedback, FeedbackNACKPLI) {
		t.Error("should not have nack pli (only nack)")
	}
}

func TestHasFeedback_Empty(t *testing.T) {
	feedback := []RTCPFeedback{}

	if HasFeedback(feedback, FeedbackNACK) {
		t.Error("expected false for empty slice")
	}
}

func TestAddFeedback(t *testing.T) {
	feedback := []RTCPFeedback{FeedbackNACK}

	// Add a new entry
	result := AddFeedback(feedback, FeedbackTransportCC)
	if len(result) != 2 {
		t.Errorf("expected 2 items after adding, got %d", len(result))
	}

	if !HasFeedback(result, FeedbackTransportCC) {
		t.Error("transport-cc should be present after adding")
	}

	// Try to add duplicate
	result = AddFeedback(result, FeedbackNACK)
	if len(result) != 2 {
		t.Errorf("expected 2 items after adding duplicate, got %d", len(result))
	}
}

func TestAddFeedback_AllowsNackAndNackPLI(t *testing.T) {
	// Should allow both nack and nack pli since they're different
	feedback := []RTCPFeedback{FeedbackNACK}

	result := AddFeedback(feedback, FeedbackNACKPLI)
	if len(result) != 2 {
		t.Errorf("expected 2 items (nack and nack pli can coexist), got %d", len(result))
	}

	if !HasFeedback(result, FeedbackNACK) {
		t.Error("nack should be present")
	}
	if !HasFeedback(result, FeedbackNACKPLI) {
		t.Error("nack pli should be present")
	}
}

func TestRemoveFeedback(t *testing.T) {
	feedback := []RTCPFeedback{FeedbackNACK, FeedbackNACKPLI, FeedbackTransportCC}

	result := RemoveFeedback(feedback, FeedbackNACKPLI)
	if len(result) != 2 {
		t.Errorf("expected 2 items after removing, got %d", len(result))
	}

	if HasFeedback(result, FeedbackNACKPLI) {
		t.Error("nack pli should not be present after removing")
	}

	// Verify other entries are still present
	if !HasFeedback(result, FeedbackNACK) {
		t.Error("nack should still be present")
	}
	if !HasFeedback(result, FeedbackTransportCC) {
		t.Error("transport-cc should still be present")
	}
}

func TestRemoveFeedback_NotPresent(t *testing.T) {
	feedback := []RTCPFeedback{FeedbackNACK}

	result := RemoveFeedback(feedback, FeedbackTransportCC)
	if len(result) != 1 {
		t.Errorf("expected 1 item when removing non-existent entry, got %d", len(result))
	}
}

func TestRemoveFeedback_Empty(t *testing.T) {
	feedback := []RTCPFeedback{}

	result := RemoveFeedback(feedback, FeedbackNACK)
	if len(result) != 0 {
		t.Errorf("expected 0 items, got %d", len(result))
	}
}

func TestValidateVideoFeedback(t *testing.T) {
	// Test with all required entries
	complete := DefaultVideoRTCPFeedback()
	missing := ValidateVideoFeedback(complete)
	if len(missing) != 0 {
		t.Errorf("expected no missing entries with default config, got %v", missing)
	}

	// Test with missing entries
	incomplete := []RTCPFeedback{FeedbackNACK, FeedbackTransportCC}
	missing = ValidateVideoFeedback(incomplete)
	if len(missing) != 3 {
		t.Errorf("expected 3 missing entries, got %d", len(missing))
	}

	// Verify which entries are missing
	expectedMissing := []RTCPFeedback{FeedbackNACKPLI, FeedbackCCMFIR, FeedbackGoogREMB}
	for _, expected := range expectedMissing {
		found := false
		for _, m := range missing {
			if m.Equals(expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s to be in missing list", expected.SDPFeedbackString())
		}
	}

	// Test with empty
	missing = ValidateVideoFeedback([]RTCPFeedback{})
	if len(missing) != 5 {
		t.Errorf("expected 5 missing entries for empty slice, got %d", len(missing))
	}
}

func TestValidateAudioFeedback(t *testing.T) {
	// Test with required entry
	complete := DefaultAudioRTCPFeedback()
	missing := ValidateAudioFeedback(complete)
	if len(missing) != 0 {
		t.Errorf("expected no missing entries with default config, got %v", missing)
	}

	// Test with missing transport-cc
	incomplete := []RTCPFeedback{FeedbackNACK}
	missing = ValidateAudioFeedback(incomplete)
	if len(missing) != 1 {
		t.Errorf("expected 1 missing entry, got %d", len(missing))
	}
	if !missing[0].Equals(FeedbackTransportCC) {
		t.Errorf("expected missing entry transport-cc, got %s", missing[0].SDPFeedbackString())
	}

	// Test with empty
	missing = ValidateAudioFeedback([]RTCPFeedback{})
	if len(missing) != 1 {
		t.Errorf("expected 1 missing entry for empty slice, got %d", len(missing))
	}
}

func TestEnsureRequiredVideoFeedback(t *testing.T) {
	// Start with empty
	feedback := []RTCPFeedback{}
	result := EnsureRequiredVideoFeedback(feedback)

	if len(result) != 5 {
		t.Errorf("expected 5 entries after ensuring, got %d", len(result))
	}

	// Validate all required entries are present
	missing := ValidateVideoFeedback(result)
	if len(missing) != 0 {
		t.Errorf("expected no missing entries after ensuring, got %v", missing)
	}

	// Start with partial
	partial := []RTCPFeedback{FeedbackNACK}
	result = EnsureRequiredVideoFeedback(partial)

	if len(result) != 5 {
		t.Errorf("expected 5 entries after ensuring partial, got %d", len(result))
	}

	// Should not duplicate existing entry
	for i, f := range result {
		for j, f2 := range result {
			if i != j && f.Equals(f2) {
				t.Errorf("duplicate feedback entry: %s", f.SDPFeedbackString())
			}
		}
	}
}

func TestEnsureRequiredAudioFeedback(t *testing.T) {
	// Start with empty
	feedback := []RTCPFeedback{}
	result := EnsureRequiredAudioFeedback(feedback)

	if len(result) != 1 {
		t.Errorf("expected 1 entry after ensuring, got %d", len(result))
	}

	if !HasFeedback(result, FeedbackTransportCC) {
		t.Error("transport-cc should be present after ensuring")
	}

	// Start with already having transport-cc
	existing := []RTCPFeedback{FeedbackTransportCC}
	result = EnsureRequiredAudioFeedback(existing)

	if len(result) != 1 {
		t.Errorf("expected 1 entry after ensuring existing, got %d", len(result))
	}
}

func TestSDPFeedbackString(t *testing.T) {
	tests := []struct {
		name     string
		feedback RTCPFeedback
		expected string
	}{
		{"nack", FeedbackNACK, "nack"},
		{"nack pli", FeedbackNACKPLI, "nack pli"},
		{"ccm fir", FeedbackCCMFIR, "ccm fir"},
		{"goog-remb", FeedbackGoogREMB, "goog-remb"},
		{"transport-cc", FeedbackTransportCC, "transport-cc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.feedback.SDPFeedbackString()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRoundTripConversion(t *testing.T) {
	// Test that converting to webrtc.RTCPFeedback and back preserves the data
	original := DefaultVideoRTCPFeedback()

	for _, fb := range original {
		webrtcFB := fb.ToWebRTCFeedback()
		roundTripped := RTCPFeedbackFromWebRTC(webrtcFB)

		if !fb.Equals(roundTripped) {
			t.Errorf("round-trip conversion failed for %s: got %v", fb.SDPFeedbackString(), roundTripped)
		}
	}
}

func TestRequiredVideoFeedback(t *testing.T) {
	required := RequiredVideoFeedback()

	if len(required) != 5 {
		t.Errorf("expected 5 required video feedback entries, got %d", len(required))
	}

	// Verify same as default
	defaults := DefaultVideoRTCPFeedback()
	for _, r := range required {
		if !HasFeedback(defaults, r) {
			t.Errorf("required feedback %s not in defaults", r.SDPFeedbackString())
		}
	}
}

func TestRequiredAudioFeedback(t *testing.T) {
	required := RequiredAudioFeedback()

	if len(required) != 1 {
		t.Errorf("expected 1 required audio feedback entry, got %d", len(required))
	}

	if !required[0].Equals(FeedbackTransportCC) {
		t.Errorf("expected transport-cc, got %s", required[0].SDPFeedbackString())
	}
}

func TestFeedbackConstants(t *testing.T) {
	// Verify the pre-defined constants match expected pion/webrtc format
	tests := []struct {
		name      string
		feedback  RTCPFeedback
		wantType  string
		wantParam string
	}{
		{"FeedbackNACK", FeedbackNACK, "nack", ""},
		{"FeedbackNACKPLI", FeedbackNACKPLI, "nack", "pli"},
		{"FeedbackCCMFIR", FeedbackCCMFIR, "ccm", "fir"},
		{"FeedbackGoogREMB", FeedbackGoogREMB, "goog-remb", ""},
		{"FeedbackTransportCC", FeedbackTransportCC, "transport-cc", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webrtcFB := tt.feedback.ToWebRTCFeedback()
			if webrtcFB.Type != tt.wantType {
				t.Errorf("Type: expected %q, got %q", tt.wantType, webrtcFB.Type)
			}
			if webrtcFB.Parameter != tt.wantParam {
				t.Errorf("Parameter: expected %q, got %q", tt.wantParam, webrtcFB.Parameter)
			}
		})
	}
}

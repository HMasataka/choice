package webrtc

import (
	"github.com/pion/webrtc/v4"
)

// RTCPFeedbackType represents the type of RTCP feedback.
// These match pion/webrtc's Type field values.
type RTCPFeedbackType string

const (
	// RTCPFeedbackTypeNACK is the NACK feedback type.
	RTCPFeedbackTypeNACK RTCPFeedbackType = "nack"
	// RTCPFeedbackTypeCCM is the CCM (Codec Control Messages) feedback type.
	RTCPFeedbackTypeCCM RTCPFeedbackType = "ccm"
	// RTCPFeedbackTypeGoogREMB is the goog-remb feedback type for bandwidth estimation.
	RTCPFeedbackTypeGoogREMB RTCPFeedbackType = "goog-remb"
	// RTCPFeedbackTypeTransportCC is the transport-cc feedback type for congestion control.
	RTCPFeedbackTypeTransportCC RTCPFeedbackType = "transport-cc"
)

// RTCPFeedbackParam represents the parameter for RTCP feedback.
// These match pion/webrtc's Parameter field values.
type RTCPFeedbackParam string

const (
	// RTCPFeedbackParamNone represents no parameter (empty string).
	RTCPFeedbackParamNone RTCPFeedbackParam = ""
	// RTCPFeedbackParamPLI is the PLI (Picture Loss Indication) parameter.
	RTCPFeedbackParamPLI RTCPFeedbackParam = "pli"
	// RTCPFeedbackParamFIR is the FIR (Full Intra Request) parameter.
	RTCPFeedbackParamFIR RTCPFeedbackParam = "fir"
)

// RTCPFeedback represents an RTCP feedback mechanism configuration.
// This structure aligns with pion/webrtc's RTCPFeedback representation.
type RTCPFeedback struct {
	// Type is the RTCP feedback type (e.g., "nack", "ccm", "goog-remb").
	Type RTCPFeedbackType
	// Parameter is the optional parameter (e.g., "pli" for nack, "fir" for ccm).
	Parameter RTCPFeedbackParam
}

// RTCPFeedbackConfig holds configuration for RTCP feedback mechanisms.
type RTCPFeedbackConfig struct {
	// VideoFeedback is the list of RTCP feedback mechanisms for video.
	VideoFeedback []RTCPFeedback
	// AudioFeedback is the list of RTCP feedback mechanisms for audio.
	AudioFeedback []RTCPFeedback
}

// Pre-defined RTCP feedback configurations matching design spec (section 3.6.3)
var (
	// FeedbackNACK represents "a=rtcp-fb:* nack" for packet retransmission.
	FeedbackNACK = RTCPFeedback{Type: RTCPFeedbackTypeNACK, Parameter: RTCPFeedbackParamNone}
	// FeedbackNACKPLI represents "a=rtcp-fb:* nack pli" for Picture Loss Indication.
	FeedbackNACKPLI = RTCPFeedback{Type: RTCPFeedbackTypeNACK, Parameter: RTCPFeedbackParamPLI}
	// FeedbackCCMFIR represents "a=rtcp-fb:* ccm fir" for Full Intra Request.
	FeedbackCCMFIR = RTCPFeedback{Type: RTCPFeedbackTypeCCM, Parameter: RTCPFeedbackParamFIR}
	// FeedbackGoogREMB represents "a=rtcp-fb:* goog-remb" for bandwidth estimation.
	FeedbackGoogREMB = RTCPFeedback{Type: RTCPFeedbackTypeGoogREMB, Parameter: RTCPFeedbackParamNone}
	// FeedbackTransportCC represents "a=rtcp-fb:* transport-cc" for congestion control.
	FeedbackTransportCC = RTCPFeedback{Type: RTCPFeedbackTypeTransportCC, Parameter: RTCPFeedbackParamNone}
)

// DefaultVideoRTCPFeedback returns the default RTCP feedback mechanisms for video.
// Per design spec (section 3.6.3):
// - nack: Packet retransmission request
// - nack pli: Picture Loss Indication
// - ccm fir: Full Intra Request
// - goog-remb: REMB bandwidth estimation
// - transport-cc: TWCC congestion control
func DefaultVideoRTCPFeedback() []RTCPFeedback {
	return []RTCPFeedback{
		FeedbackNACK,
		FeedbackNACKPLI,
		FeedbackCCMFIR,
		FeedbackGoogREMB,
		FeedbackTransportCC,
	}
}

// DefaultAudioRTCPFeedback returns the default RTCP feedback mechanisms for audio.
// Per design spec (section 3.6.3):
// - transport-cc: TWCC congestion control (for audio and video)
func DefaultAudioRTCPFeedback() []RTCPFeedback {
	return []RTCPFeedback{
		FeedbackTransportCC,
	}
}

// DefaultRTCPFeedbackConfig returns the default RTCP feedback configuration.
func DefaultRTCPFeedbackConfig() RTCPFeedbackConfig {
	return RTCPFeedbackConfig{
		VideoFeedback: DefaultVideoRTCPFeedback(),
		AudioFeedback: DefaultAudioRTCPFeedback(),
	}
}

// ToWebRTCFeedback converts RTCPFeedback to webrtc.RTCPFeedback.
func (f RTCPFeedback) ToWebRTCFeedback() webrtc.RTCPFeedback {
	return webrtc.RTCPFeedback{
		Type:      string(f.Type),
		Parameter: string(f.Parameter),
	}
}

// ToWebRTCFeedbackSlice converts a slice of RTCPFeedback to webrtc.RTCPFeedback slice.
func ToWebRTCFeedbackSlice(feedback []RTCPFeedback) []webrtc.RTCPFeedback {
	result := make([]webrtc.RTCPFeedback, len(feedback))
	for i, f := range feedback {
		result[i] = f.ToWebRTCFeedback()
	}
	return result
}

// RTCPFeedbackFromWebRTC converts webrtc.RTCPFeedback to RTCPFeedback.
func RTCPFeedbackFromWebRTC(f webrtc.RTCPFeedback) RTCPFeedback {
	return RTCPFeedback{
		Type:      RTCPFeedbackType(f.Type),
		Parameter: RTCPFeedbackParam(f.Parameter),
	}
}

// Equals checks if two RTCPFeedback are equal (same Type and Parameter).
func (f RTCPFeedback) Equals(other RTCPFeedback) bool {
	return f.Type == other.Type && f.Parameter == other.Parameter
}

// HasFeedback checks if the given feedback is present in the slice.
// This compares both Type and Parameter for exact match.
func HasFeedback(feedback []RTCPFeedback, target RTCPFeedback) bool {
	for _, f := range feedback {
		if f.Equals(target) {
			return true
		}
	}
	return false
}

// AddFeedback adds a feedback to the slice if it doesn't exist.
// This compares both Type and Parameter for deduplication.
func AddFeedback(feedback []RTCPFeedback, target RTCPFeedback) []RTCPFeedback {
	if HasFeedback(feedback, target) {
		return feedback
	}
	return append(feedback, target)
}

// RemoveFeedback removes a feedback from the slice.
// This compares both Type and Parameter for exact match.
func RemoveFeedback(feedback []RTCPFeedback, target RTCPFeedback) []RTCPFeedback {
	result := make([]RTCPFeedback, 0, len(feedback))
	for _, f := range feedback {
		if !f.Equals(target) {
			result = append(result, f)
		}
	}
	return result
}

// RequiredVideoFeedback returns all required RTCP feedback for video codecs.
func RequiredVideoFeedback() []RTCPFeedback {
	return []RTCPFeedback{
		FeedbackNACK,
		FeedbackNACKPLI,
		FeedbackCCMFIR,
		FeedbackGoogREMB,
		FeedbackTransportCC,
	}
}

// RequiredAudioFeedback returns all required RTCP feedback for audio codecs.
func RequiredAudioFeedback() []RTCPFeedback {
	return []RTCPFeedback{
		FeedbackTransportCC,
	}
}

// ValidateVideoFeedback validates that all required video RTCP feedback mechanisms are present.
// Returns missing feedback if any are not present.
func ValidateVideoFeedback(feedback []RTCPFeedback) []RTCPFeedback {
	var missing []RTCPFeedback
	for _, required := range RequiredVideoFeedback() {
		if !HasFeedback(feedback, required) {
			missing = append(missing, required)
		}
	}
	return missing
}

// ValidateAudioFeedback validates that all required audio RTCP feedback mechanisms are present.
// Returns missing feedback if any are not present.
func ValidateAudioFeedback(feedback []RTCPFeedback) []RTCPFeedback {
	var missing []RTCPFeedback
	for _, required := range RequiredAudioFeedback() {
		if !HasFeedback(feedback, required) {
			missing = append(missing, required)
		}
	}
	return missing
}

// EnsureRequiredVideoFeedback ensures all required video RTCP feedback mechanisms are present.
// Returns the updated feedback slice with any missing required mechanisms added.
func EnsureRequiredVideoFeedback(feedback []RTCPFeedback) []RTCPFeedback {
	for _, required := range RequiredVideoFeedback() {
		feedback = AddFeedback(feedback, required)
	}
	return feedback
}

// EnsureRequiredAudioFeedback ensures all required audio RTCP feedback mechanisms are present.
// Returns the updated feedback slice with any missing required mechanisms added.
func EnsureRequiredAudioFeedback(feedback []RTCPFeedback) []RTCPFeedback {
	for _, required := range RequiredAudioFeedback() {
		feedback = AddFeedback(feedback, required)
	}
	return feedback
}

// SDPFeedbackString returns the SDP representation of the feedback (e.g., "nack pli").
// This is useful for logging and debugging.
func (f RTCPFeedback) SDPFeedbackString() string {
	if f.Parameter == RTCPFeedbackParamNone {
		return string(f.Type)
	}
	return string(f.Type) + " " + string(f.Parameter)
}

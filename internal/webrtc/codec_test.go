package webrtc

import (
	"errors"
	"strings"
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestDefaultCodecConfig(t *testing.T) {
	config := DefaultCodecConfig()

	// Test video codecs
	t.Run("video codecs", func(t *testing.T) {
		if len(config.VideoCodecs) != 3 {
			t.Errorf("expected 3 video codecs, got %d", len(config.VideoCodecs))
		}

		// Check VP8 is first (highest priority)
		if config.VideoCodecs[0].Name != CodecVP8 {
			t.Errorf("expected first video codec to be VP8, got %s", config.VideoCodecs[0].Name)
		}
		if config.VideoCodecs[0].Priority != 1 {
			t.Errorf("expected VP8 priority to be 1, got %d", config.VideoCodecs[0].Priority)
		}

		// Check H.264 is second
		if config.VideoCodecs[1].Name != CodecH264 {
			t.Errorf("expected second video codec to be H264, got %s", config.VideoCodecs[1].Name)
		}
		if config.VideoCodecs[1].Priority != 2 {
			t.Errorf("expected H264 priority to be 2, got %d", config.VideoCodecs[1].Priority)
		}

		// Check VP9 is third
		if config.VideoCodecs[2].Name != CodecVP9 {
			t.Errorf("expected third video codec to be VP9, got %s", config.VideoCodecs[2].Name)
		}
		if config.VideoCodecs[2].Priority != 3 {
			t.Errorf("expected VP9 priority to be 3, got %d", config.VideoCodecs[2].Priority)
		}
	})

	// Test audio codecs
	t.Run("audio codecs", func(t *testing.T) {
		if len(config.AudioCodecs) != 1 {
			t.Errorf("expected 1 audio codec, got %d", len(config.AudioCodecs))
		}

		if config.AudioCodecs[0].Name != CodecOpus {
			t.Errorf("expected audio codec to be opus, got %s", config.AudioCodecs[0].Name)
		}
		if config.AudioCodecs[0].ClockRate != 48000 {
			t.Errorf("expected Opus clock rate to be 48000, got %d", config.AudioCodecs[0].ClockRate)
		}
		if config.AudioCodecs[0].Channels != 2 {
			t.Errorf("expected Opus channels to be 2, got %d", config.AudioCodecs[0].Channels)
		}
	})
}

func TestDefaultCodecConfigWithG711(t *testing.T) {
	config := DefaultCodecConfigWithG711()

	if len(config.AudioCodecs) != 3 {
		t.Errorf("expected 3 audio codecs, got %d", len(config.AudioCodecs))
	}

	// Check Opus is first
	if config.AudioCodecs[0].Name != CodecOpus {
		t.Errorf("expected first audio codec to be opus, got %s", config.AudioCodecs[0].Name)
	}

	// Check G.711 PCMU
	if config.AudioCodecs[1].Name != CodecG711PCMU {
		t.Errorf("expected second audio codec to be PCMU, got %s", config.AudioCodecs[1].Name)
	}
	if config.AudioCodecs[1].ClockRate != 8000 {
		t.Errorf("expected PCMU clock rate to be 8000, got %d", config.AudioCodecs[1].ClockRate)
	}

	// Check G.711 PCMA
	if config.AudioCodecs[2].Name != CodecG711PCMA {
		t.Errorf("expected third audio codec to be PCMA, got %s", config.AudioCodecs[2].Name)
	}
}

func TestH264ProfileConfig(t *testing.T) {
	t.Run("high profile", func(t *testing.T) {
		profile := DefaultH264HighProfile()

		if profile.ProfileLevelID != H264ProfileHighLevel50 {
			t.Errorf("expected profile-level-id to be %s, got %s", H264ProfileHighLevel50, profile.ProfileLevelID)
		}
		if profile.PacketizationMode != 1 {
			t.Errorf("expected packetization-mode to be 1, got %d", profile.PacketizationMode)
		}
		if profile.LevelAsymmetryAllowed != 1 {
			t.Errorf("expected level-asymmetry-allowed to be 1, got %d", profile.LevelAsymmetryAllowed)
		}

		fmtp := profile.BuildFMTPLine()
		if !strings.Contains(fmtp, "profile-level-id=640032") {
			t.Errorf("fmtp line missing profile-level-id=640032: %s", fmtp)
		}
		if !strings.Contains(fmtp, "packetization-mode=1") {
			t.Errorf("fmtp line missing packetization-mode=1: %s", fmtp)
		}
		if !strings.Contains(fmtp, "level-asymmetry-allowed=1") {
			t.Errorf("fmtp line missing level-asymmetry-allowed=1: %s", fmtp)
		}
	})

	t.Run("baseline profile", func(t *testing.T) {
		profile := DefaultH264BaselineProfile()

		if profile.ProfileLevelID != H264ProfileConstrainedBaselineLevel31 {
			t.Errorf("expected profile-level-id to be %s, got %s", H264ProfileConstrainedBaselineLevel31, profile.ProfileLevelID)
		}

		fmtp := profile.BuildFMTPLine()
		if !strings.Contains(fmtp, "profile-level-id=42e01f") {
			t.Errorf("fmtp line missing profile-level-id=42e01f: %s", fmtp)
		}
	})
}

func TestOpusFMTPParams(t *testing.T) {
	params := DefaultOpusFMTP()

	if params.MinPTime != 10 {
		t.Errorf("expected minptime to be 10, got %d", params.MinPTime)
	}
	if params.UseInbandFEC != 1 {
		t.Errorf("expected useinbandfec to be 1, got %d", params.UseInbandFEC)
	}
	if params.Stereo != 1 {
		t.Errorf("expected stereo to be 1, got %d", params.Stereo)
	}

	fmtp := params.BuildFMTPLine()
	if !strings.Contains(fmtp, "minptime=10") {
		t.Errorf("fmtp line missing minptime=10: %s", fmtp)
	}
	if !strings.Contains(fmtp, "useinbandfec=1") {
		t.Errorf("fmtp line missing useinbandfec=1: %s", fmtp)
	}
	if !strings.Contains(fmtp, "stereo=1") {
		t.Errorf("fmtp line missing stereo=1: %s", fmtp)
	}
}

func TestH264Profiles(t *testing.T) {
	config := DefaultCodecConfig()

	h264 := config.GetVideoCodecByName(CodecH264)
	if h264 == nil {
		t.Fatal("H.264 codec not found")
	}

	if len(h264.Profiles) != 2 {
		t.Errorf("expected 2 H.264 profiles, got %d", len(h264.Profiles))
	}

	// Check High Profile is first
	if h264.Profiles[0].ProfileLevelID != H264ProfileHighLevel50 {
		t.Errorf("expected first profile to be High Profile, got %s", h264.Profiles[0].ProfileLevelID)
	}

	// Check Constrained Baseline is second
	if h264.Profiles[1].ProfileLevelID != H264ProfileConstrainedBaselineLevel31 {
		t.Errorf("expected second profile to be Constrained Baseline, got %s", h264.Profiles[1].ProfileLevelID)
	}
}

func TestVideoCodecRTCPFeedback(t *testing.T) {
	config := DefaultCodecConfig()

	for _, vc := range config.VideoCodecs {
		t.Run(string(vc.Name), func(t *testing.T) {
			// All video codecs should have required RTCP feedback
			if !HasFeedback(vc.RTCPFeedback, FeedbackNACK) {
				t.Error("missing NACK feedback")
			}
			if !HasFeedback(vc.RTCPFeedback, FeedbackNACKPLI) {
				t.Error("missing NACK PLI feedback")
			}
			if !HasFeedback(vc.RTCPFeedback, FeedbackCCMFIR) {
				t.Error("missing CCM FIR feedback")
			}
			if !HasFeedback(vc.RTCPFeedback, FeedbackGoogREMB) {
				t.Error("missing goog-remb feedback")
			}
			if !HasFeedback(vc.RTCPFeedback, FeedbackTransportCC) {
				t.Error("missing transport-cc feedback")
			}
		})
	}
}

func TestAudioCodecRTCPFeedback(t *testing.T) {
	config := DefaultCodecConfig()

	for _, ac := range config.AudioCodecs {
		t.Run(string(ac.Name), func(t *testing.T) {
			// Audio codecs should have transport-cc feedback
			if !HasFeedback(ac.RTCPFeedback, FeedbackTransportCC) {
				t.Error("missing transport-cc feedback")
			}
		})
	}
}

func TestRegisterCodecs(t *testing.T) {
	m := &webrtc.MediaEngine{}
	config := DefaultCodecConfig()

	err := RegisterCodecs(m, config)
	if err != nil {
		t.Fatalf("RegisterCodecs failed: %v", err)
	}

	// Verify codecs were registered by checking the engine can be used
	// We can't directly inspect the MediaEngine, but we can verify no errors occurred
}

func TestNewMediaEngineWithDefaults(t *testing.T) {
	m, err := NewMediaEngineWithDefaults()
	if err != nil {
		t.Fatalf("NewMediaEngineWithDefaults failed: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil MediaEngine")
	}
}

func TestNewMediaEngineWithConfig(t *testing.T) {
	config := DefaultCodecConfig()

	// Remove VP9 to test custom config
	config.RemoveVideoCodec(CodecVP9)

	m, err := NewMediaEngineWithConfig(config)
	if err != nil {
		t.Fatalf("NewMediaEngineWithConfig failed: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil MediaEngine")
	}
}

func TestCodecConfigHelpers(t *testing.T) {
	config := DefaultCodecConfig()

	t.Run("GetVideoCodecNames", func(t *testing.T) {
		names := config.GetVideoCodecNames()
		if len(names) != 3 {
			t.Errorf("expected 3 video codec names, got %d", len(names))
		}
	})

	t.Run("GetAudioCodecNames", func(t *testing.T) {
		names := config.GetAudioCodecNames()
		if len(names) != 1 {
			t.Errorf("expected 1 audio codec name, got %d", len(names))
		}
	})

	t.Run("HasVideoCodec", func(t *testing.T) {
		if !config.HasVideoCodec(CodecVP8) {
			t.Error("expected to have VP8 codec")
		}
		if !config.HasVideoCodec(CodecH264) {
			t.Error("expected to have H264 codec")
		}
		if config.HasVideoCodec(CodecAV1) {
			t.Error("expected not to have AV1 codec")
		}
	})

	t.Run("HasAudioCodec", func(t *testing.T) {
		if !config.HasAudioCodec(CodecOpus) {
			t.Error("expected to have Opus codec")
		}
		if config.HasAudioCodec(CodecG711PCMU) {
			t.Error("expected not to have PCMU codec")
		}
	})

	t.Run("GetVideoCodecByName", func(t *testing.T) {
		vp8 := config.GetVideoCodecByName(CodecVP8)
		if vp8 == nil {
			t.Fatal("expected to find VP8 codec")
		}
		if vp8.Name != CodecVP8 {
			t.Errorf("expected VP8, got %s", vp8.Name)
		}

		av1 := config.GetVideoCodecByName(CodecAV1)
		if av1 != nil {
			t.Error("expected not to find AV1 codec")
		}
	})

	t.Run("GetAudioCodecByName", func(t *testing.T) {
		opus := config.GetAudioCodecByName(CodecOpus)
		if opus == nil {
			t.Fatal("expected to find Opus codec")
		}
		if opus.Name != CodecOpus {
			t.Errorf("expected opus, got %s", opus.Name)
		}
	})
}

func TestSetVideoCodecPriority(t *testing.T) {
	config := DefaultCodecConfig()

	// Initially VP8 is priority 1, H264 is priority 2
	if config.VideoCodecs[0].Name != CodecVP8 {
		t.Fatal("expected VP8 to be first")
	}

	// Change VP8 to lower priority (higher number)
	config.SetVideoCodecPriority(CodecVP8, 10)

	// Now VP8 should be last
	if config.VideoCodecs[len(config.VideoCodecs)-1].Name != CodecVP8 {
		t.Errorf("expected VP8 to be last after priority change, got %s", config.VideoCodecs[len(config.VideoCodecs)-1].Name)
	}
}

func TestAddRemoveVideoCodec(t *testing.T) {
	config := DefaultCodecConfig()
	initialCount := len(config.VideoCodecs)

	// Add AV1 codec
	config.AddVideoCodec(VideoCodecConfig{
		Name:         CodecAV1,
		Priority:     4,
		MimeType:     "video/AV1",
		ClockRate:    90000,
		RTCPFeedback: DefaultVideoRTCPFeedback(),
	})

	if len(config.VideoCodecs) != initialCount+1 {
		t.Errorf("expected %d video codecs after add, got %d", initialCount+1, len(config.VideoCodecs))
	}

	if !config.HasVideoCodec(CodecAV1) {
		t.Error("expected to have AV1 after add")
	}

	// Remove AV1 codec
	config.RemoveVideoCodec(CodecAV1)

	if len(config.VideoCodecs) != initialCount {
		t.Errorf("expected %d video codecs after remove, got %d", initialCount, len(config.VideoCodecs))
	}

	if config.HasVideoCodec(CodecAV1) {
		t.Error("expected not to have AV1 after remove")
	}
}

func TestAddRemoveAudioCodec(t *testing.T) {
	config := DefaultCodecConfig()
	initialCount := len(config.AudioCodecs)

	// Add G.711 PCMU
	config.AddAudioCodec(AudioCodecConfig{
		Name:         CodecG711PCMU,
		Priority:     2,
		MimeType:     webrtc.MimeTypePCMU,
		ClockRate:    8000,
		Channels:     1,
		RTCPFeedback: DefaultAudioRTCPFeedback(),
	})

	if len(config.AudioCodecs) != initialCount+1 {
		t.Errorf("expected %d audio codecs after add, got %d", initialCount+1, len(config.AudioCodecs))
	}

	// Remove G.711 PCMU
	config.RemoveAudioCodec(CodecG711PCMU)

	if len(config.AudioCodecs) != initialCount {
		t.Errorf("expected %d audio codecs after remove, got %d", initialCount, len(config.AudioCodecs))
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{-1, "-1"},
		{-123, "-123"},
	}

	for _, tt := range tests {
		result := itoa(tt.input)
		if result != tt.expected {
			t.Errorf("itoa(%d) = %s, expected %s", tt.input, result, tt.expected)
		}
	}
}

func TestVideoCodecMimeTypes(t *testing.T) {
	config := DefaultCodecConfig()

	tests := []struct {
		name     VideoCodecName
		expected string
	}{
		{CodecVP8, webrtc.MimeTypeVP8},
		{CodecH264, webrtc.MimeTypeH264},
		{CodecVP9, webrtc.MimeTypeVP9},
	}

	for _, tt := range tests {
		vc := config.GetVideoCodecByName(tt.name)
		if vc == nil {
			t.Errorf("codec %s not found", tt.name)
			continue
		}
		if vc.MimeType != tt.expected {
			t.Errorf("expected %s mime type to be %s, got %s", tt.name, tt.expected, vc.MimeType)
		}
	}
}

func TestAudioCodecMimeTypes(t *testing.T) {
	config := DefaultCodecConfigWithG711()

	tests := []struct {
		name     AudioCodecName
		expected string
	}{
		{CodecOpus, webrtc.MimeTypeOpus},
		{CodecG711PCMU, webrtc.MimeTypePCMU},
		{CodecG711PCMA, webrtc.MimeTypePCMA},
	}

	for _, tt := range tests {
		ac := config.GetAudioCodecByName(tt.name)
		if ac == nil {
			t.Errorf("codec %s not found", tt.name)
			continue
		}
		if ac.MimeType != tt.expected {
			t.Errorf("expected %s mime type to be %s, got %s", tt.name, tt.expected, ac.MimeType)
		}
	}
}

func TestValidateCodecConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := DefaultCodecConfig()
		err := ValidateCodecConfig(config)
		if err != nil {
			t.Errorf("expected no error for valid config, got %v", err)
		}
	})

	t.Run("no video codecs", func(t *testing.T) {
		config := CodecConfig{
			VideoCodecs: []VideoCodecConfig{},
			AudioCodecs: []AudioCodecConfig{
				{Name: CodecOpus, MimeType: webrtc.MimeTypeOpus},
			},
		}
		err := ValidateCodecConfig(config)
		if !errors.Is(err, ErrNoVideoCodecs) {
			t.Errorf("expected ErrNoVideoCodecs, got %v", err)
		}
	})

	t.Run("no audio codecs", func(t *testing.T) {
		config := CodecConfig{
			VideoCodecs: []VideoCodecConfig{
				{Name: CodecVP8, MimeType: webrtc.MimeTypeVP8},
			},
			AudioCodecs: []AudioCodecConfig{},
		}
		err := ValidateCodecConfig(config)
		if !errors.Is(err, ErrNoAudioCodecs) {
			t.Errorf("expected ErrNoAudioCodecs, got %v", err)
		}
	})

	t.Run("unknown video codec", func(t *testing.T) {
		config := CodecConfig{
			VideoCodecs: []VideoCodecConfig{
				{Name: "UNKNOWN", MimeType: "video/unknown"},
			},
			AudioCodecs: []AudioCodecConfig{
				{Name: CodecOpus, MimeType: webrtc.MimeTypeOpus},
			},
		}
		err := ValidateCodecConfig(config)
		if !errors.Is(err, ErrUnknownVideoCodec) {
			t.Errorf("expected ErrUnknownVideoCodec, got %v", err)
		}
	})

	t.Run("unknown audio codec", func(t *testing.T) {
		config := CodecConfig{
			VideoCodecs: []VideoCodecConfig{
				{Name: CodecVP8, MimeType: webrtc.MimeTypeVP8},
			},
			AudioCodecs: []AudioCodecConfig{
				{Name: "UNKNOWN", MimeType: "audio/unknown"},
			},
		}
		err := ValidateCodecConfig(config)
		if !errors.Is(err, ErrUnknownAudioCodec) {
			t.Errorf("expected ErrUnknownAudioCodec, got %v", err)
		}
	})

	t.Run("H264 with no profiles", func(t *testing.T) {
		config := CodecConfig{
			VideoCodecs: []VideoCodecConfig{
				{Name: CodecH264, MimeType: webrtc.MimeTypeH264, Profiles: []H264ProfileConfig{}},
			},
			AudioCodecs: []AudioCodecConfig{
				{Name: CodecOpus, MimeType: webrtc.MimeTypeOpus},
			},
		}
		err := ValidateCodecConfig(config)
		if !errors.Is(err, ErrH264NoProfiles) {
			t.Errorf("expected ErrH264NoProfiles, got %v", err)
		}
	})
}

func TestSetAudioCodecPriority(t *testing.T) {
	config := DefaultCodecConfigWithG711()

	// Initially Opus is priority 1, PCMU is priority 2, PCMA is priority 3
	if config.AudioCodecs[0].Name != CodecOpus {
		t.Fatal("expected Opus to be first")
	}

	// Change PCMU to highest priority
	config.SetAudioCodecPriority(CodecG711PCMU, 0)

	// Now PCMU should be first
	if config.AudioCodecs[0].Name != CodecG711PCMU {
		t.Errorf("expected PCMU to be first after priority change, got %s", config.AudioCodecs[0].Name)
	}
}

func TestSortByPriority(t *testing.T) {
	config := CodecConfig{
		VideoCodecs: []VideoCodecConfig{
			{Name: CodecVP9, Priority: 3, MimeType: webrtc.MimeTypeVP9},
			{Name: CodecVP8, Priority: 1, MimeType: webrtc.MimeTypeVP8},
			{Name: CodecH264, Priority: 2, MimeType: webrtc.MimeTypeH264, Profiles: []H264ProfileConfig{DefaultH264HighProfile()}},
		},
		AudioCodecs: []AudioCodecConfig{
			{Name: CodecG711PCMU, Priority: 2, MimeType: webrtc.MimeTypePCMU},
			{Name: CodecOpus, Priority: 1, MimeType: webrtc.MimeTypeOpus},
		},
	}

	config.SortByPriority()

	// Check video codec order
	if config.VideoCodecs[0].Name != CodecVP8 {
		t.Errorf("expected VP8 first, got %s", config.VideoCodecs[0].Name)
	}
	if config.VideoCodecs[1].Name != CodecH264 {
		t.Errorf("expected H264 second, got %s", config.VideoCodecs[1].Name)
	}
	if config.VideoCodecs[2].Name != CodecVP9 {
		t.Errorf("expected VP9 third, got %s", config.VideoCodecs[2].Name)
	}

	// Check audio codec order
	if config.AudioCodecs[0].Name != CodecOpus {
		t.Errorf("expected Opus first, got %s", config.AudioCodecs[0].Name)
	}
	if config.AudioCodecs[1].Name != CodecG711PCMU {
		t.Errorf("expected PCMU second, got %s", config.AudioCodecs[1].Name)
	}
}

func TestRegisterCodecsValidation(t *testing.T) {
	t.Run("empty video codecs", func(t *testing.T) {
		m := &webrtc.MediaEngine{}
		config := CodecConfig{
			VideoCodecs: []VideoCodecConfig{},
			AudioCodecs: []AudioCodecConfig{
				{Name: CodecOpus, MimeType: webrtc.MimeTypeOpus},
			},
		}
		err := RegisterCodecs(m, config)
		if !errors.Is(err, ErrNoVideoCodecs) {
			t.Errorf("expected ErrNoVideoCodecs, got %v", err)
		}
	})

	t.Run("H264 without profiles", func(t *testing.T) {
		m := &webrtc.MediaEngine{}
		config := CodecConfig{
			VideoCodecs: []VideoCodecConfig{
				{Name: CodecH264, MimeType: webrtc.MimeTypeH264, Profiles: nil},
			},
			AudioCodecs: []AudioCodecConfig{
				{Name: CodecOpus, MimeType: webrtc.MimeTypeOpus},
			},
		}
		err := RegisterCodecs(m, config)
		if !errors.Is(err, ErrH264NoProfiles) {
			t.Errorf("expected ErrH264NoProfiles, got %v", err)
		}
	})
}

func TestCodecConfigCopy(t *testing.T) {
	original := DefaultCodecConfig()

	// Make a copy
	copied := original.Copy()

	// Verify the copy has the same values
	if len(copied.VideoCodecs) != len(original.VideoCodecs) {
		t.Errorf("expected %d video codecs, got %d", len(original.VideoCodecs), len(copied.VideoCodecs))
	}
	if len(copied.AudioCodecs) != len(original.AudioCodecs) {
		t.Errorf("expected %d audio codecs, got %d", len(original.AudioCodecs), len(copied.AudioCodecs))
	}

	// Modify the copy and verify original is unchanged
	copied.VideoCodecs[0].Priority = 999

	if original.VideoCodecs[0].Priority == 999 {
		t.Error("modifying copy affected original - copy is not independent")
	}
}

func TestRegisterCodecsSortsByPriority(t *testing.T) {
	// Create config with codecs in wrong priority order
	config := CodecConfig{
		VideoCodecs: []VideoCodecConfig{
			{Name: CodecVP9, Priority: 3, MimeType: webrtc.MimeTypeVP9, ClockRate: 90000, RTCPFeedback: DefaultVideoRTCPFeedback()},
			{Name: CodecVP8, Priority: 1, MimeType: webrtc.MimeTypeVP8, ClockRate: 90000, RTCPFeedback: DefaultVideoRTCPFeedback()},
		},
		AudioCodecs: []AudioCodecConfig{
			{Name: CodecOpus, Priority: 1, MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, RTCPFeedback: DefaultAudioRTCPFeedback()},
		},
	}

	// Verify original order before RegisterCodecs
	if config.VideoCodecs[0].Name != CodecVP9 {
		t.Fatal("precondition failed: expected VP9 first in unsorted config")
	}

	m := &webrtc.MediaEngine{}
	err := RegisterCodecs(m, config)
	if err != nil {
		t.Fatalf("RegisterCodecs failed: %v", err)
	}

	// Verify original config is not mutated
	if config.VideoCodecs[0].Name != CodecVP9 {
		t.Error("RegisterCodecs mutated the input config")
	}
}

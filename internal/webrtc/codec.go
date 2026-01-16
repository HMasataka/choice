package webrtc

import (
	"errors"

	"github.com/pion/webrtc/v4"
)

var (
	// ErrUnknownVideoCodec is returned when an unknown video codec is specified.
	ErrUnknownVideoCodec = errors.New("unknown video codec")
	// ErrUnknownAudioCodec is returned when an unknown audio codec is specified.
	ErrUnknownAudioCodec = errors.New("unknown audio codec")
	// ErrH264NoProfiles is returned when H.264 codec has no profiles configured.
	ErrH264NoProfiles = errors.New("H.264 codec requires at least one profile")
	// ErrNoVideoCodecs is returned when no video codecs are configured.
	ErrNoVideoCodecs = errors.New("at least one video codec is required")
	// ErrNoAudioCodecs is returned when no audio codecs are configured.
	ErrNoAudioCodecs = errors.New("at least one audio codec is required")
)

// VideoCodecName represents supported video codec names.
type VideoCodecName string

const (
	// CodecVP8 is the VP8 video codec.
	CodecVP8 VideoCodecName = "VP8"
	// CodecVP9 is the VP9 video codec.
	// VP9 supports SVC (Scalable Video Coding) with spatial and temporal layers.
	CodecVP9 VideoCodecName = "VP9"
	// CodecH264 is the H.264 video codec.
	CodecH264 VideoCodecName = "H264"
	// CodecAV1 is the AV1 video codec.
	// AV1 supports SVC (Scalable Video Coding) with spatial and temporal layers.
	CodecAV1 VideoCodecName = "AV1"
)

// AudioCodecName represents supported audio codec names.
type AudioCodecName string

const (
	// CodecOpus is the Opus audio codec.
	CodecOpus AudioCodecName = "opus"
	// CodecG711PCMU is the G.711 µ-law audio codec.
	CodecG711PCMU AudioCodecName = "PCMU"
	// CodecG711PCMA is the G.711 A-law audio codec.
	CodecG711PCMA AudioCodecName = "PCMA"
)

// H264Profile represents H.264 profile levels.
type H264Profile string

const (
	// H264ProfileHighLevel50 is High Profile Level 5.0 (640032).
	// Suitable for high quality (1080p30), desktop use.
	H264ProfileHighLevel50 H264Profile = "640032"
	// H264ProfileConstrainedBaselineLevel31 is Constrained Baseline Level 3.1 (42e01f).
	// Safari/mobile compatible, suitable for low-spec devices.
	H264ProfileConstrainedBaselineLevel31 H264Profile = "42e01f"
)

// VideoCodecConfig holds configuration for a video codec.
type VideoCodecConfig struct {
	// Name is the codec name.
	Name VideoCodecName
	// Priority is the codec priority (lower is higher priority).
	Priority int
	// MimeType is the MIME type of the codec.
	MimeType string
	// ClockRate is the clock rate in Hz.
	ClockRate uint32
	// RTCPFeedback is the list of RTCP feedback mechanisms.
	RTCPFeedback []RTCPFeedback
	// Profiles contains H.264 specific profile configurations.
	// Only used when Name is CodecH264.
	Profiles []H264ProfileConfig
	// SVCConfig contains SVC-specific configuration for VP9/AV1.
	// Only used when Name is CodecVP9 or CodecAV1.
	SVCConfig *SVCCodecConfig
}

// SVCCodecConfig holds SVC-specific configuration for VP9/AV1 codecs.
type SVCCodecConfig struct {
	// Enabled indicates if SVC is enabled for this codec.
	Enabled bool
	// ScalabilityMode is the scalability mode string (e.g., "L3T3").
	// L<n>T<m> means n spatial layers and m temporal layers.
	ScalabilityMode string
}

// H264ProfileConfig holds H.264 specific profile configuration.
type H264ProfileConfig struct {
	// ProfileLevelID is the profile-level-id (e.g., "640032", "42e01f").
	ProfileLevelID H264Profile
	// PacketizationMode is the packetization mode (1 for non-interleaved).
	PacketizationMode int
	// LevelAsymmetryAllowed indicates if level asymmetry is allowed.
	LevelAsymmetryAllowed int
}

// AudioCodecConfig holds configuration for an audio codec.
type AudioCodecConfig struct {
	// Name is the codec name.
	Name AudioCodecName
	// Priority is the codec priority (lower is higher priority).
	Priority int
	// MimeType is the MIME type of the codec.
	MimeType string
	// ClockRate is the clock rate in Hz.
	ClockRate uint32
	// Channels is the number of audio channels.
	Channels uint16
	// RTCPFeedback is the list of RTCP feedback mechanisms.
	RTCPFeedback []RTCPFeedback
	// FMTPParams contains codec-specific parameters.
	FMTPParams OpusFMTPParams
}

// OpusFMTPParams holds Opus codec FMTP parameters.
type OpusFMTPParams struct {
	// MinPTime is the minimum packetization time in ms.
	MinPTime int
	// UseInbandFEC enables in-band forward error correction.
	UseInbandFEC int
	// Stereo enables stereo support.
	Stereo int
}

// CodecConfig holds the complete codec configuration.
type CodecConfig struct {
	// VideoCodecs is the list of video codecs in priority order.
	VideoCodecs []VideoCodecConfig
	// AudioCodecs is the list of audio codecs in priority order.
	AudioCodecs []AudioCodecConfig
}

// DefaultOpusFMTP returns the default Opus FMTP parameters per spec.
// Per design spec (section 3.6.4):
// - minptime=10: minimum packetization time
// - useinbandfec=1: enable in-band FEC
// - stereo=1: enable stereo
func DefaultOpusFMTP() OpusFMTPParams {
	return OpusFMTPParams{
		MinPTime:     10,
		UseInbandFEC: 1,
		Stereo:       1,
	}
}

// DefaultH264HighProfile returns the default H.264 High Profile configuration.
// Per design spec (section 3.6.4):
// - profile-level-id=640032 (High Profile Level 5.0)
// - packetization-mode=1 (non-interleaved)
// - level-asymmetry-allowed=1
func DefaultH264HighProfile() H264ProfileConfig {
	return H264ProfileConfig{
		ProfileLevelID:        H264ProfileHighLevel50,
		PacketizationMode:     1,
		LevelAsymmetryAllowed: 1,
	}
}

// DefaultH264BaselineProfile returns the default H.264 Constrained Baseline Profile.
// Per design spec (section 3.6.4):
// - profile-level-id=42e01f (Constrained Baseline Level 3.1)
// - packetization-mode=1 (non-interleaved)
// - level-asymmetry-allowed=1
func DefaultH264BaselineProfile() H264ProfileConfig {
	return H264ProfileConfig{
		ProfileLevelID:        H264ProfileConstrainedBaselineLevel31,
		PacketizationMode:     1,
		LevelAsymmetryAllowed: 1,
	}
}

// DefaultCodecConfig returns the default codec configuration per spec.
// Per requirements.md (section 2.6) and design.md (section 3.6.4):
// Video codecs priority: VP8 > H.264 > VP9
// Audio codecs: Opus (required), G.711 (optional for legacy)
func DefaultCodecConfig() CodecConfig {
	return CodecConfig{
		VideoCodecs: []VideoCodecConfig{
			{
				Name:         CodecVP8,
				Priority:     1,
				MimeType:     webrtc.MimeTypeVP8,
				ClockRate:    90000,
				RTCPFeedback: DefaultVideoRTCPFeedback(),
			},
			{
				Name:         CodecH264,
				Priority:     2,
				MimeType:     webrtc.MimeTypeH264,
				ClockRate:    90000,
				RTCPFeedback: DefaultVideoRTCPFeedback(),
				Profiles: []H264ProfileConfig{
					DefaultH264HighProfile(),
					DefaultH264BaselineProfile(),
				},
			},
			{
				Name:         CodecVP9,
				Priority:     3,
				MimeType:     webrtc.MimeTypeVP9,
				ClockRate:    90000,
				RTCPFeedback: DefaultVideoRTCPFeedback(),
			},
		},
		AudioCodecs: []AudioCodecConfig{
			{
				Name:         CodecOpus,
				Priority:     1,
				MimeType:     webrtc.MimeTypeOpus,
				ClockRate:    48000,
				Channels:     2,
				RTCPFeedback: DefaultAudioRTCPFeedback(),
				FMTPParams:   DefaultOpusFMTP(),
			},
		},
	}
}

// DefaultCodecConfigWithSVC returns the default codec configuration with SVC enabled for VP9/AV1.
// Per design.md section 4.1: SVC is optional for VP9/AV1 codecs.
func DefaultCodecConfigWithSVC(scalabilityMode string) CodecConfig {
	if scalabilityMode == "" {
		scalabilityMode = "L3T3"
	}
	return CodecConfig{
		VideoCodecs: []VideoCodecConfig{
			{
				Name:         CodecVP8,
				Priority:     3, // Lower priority when SVC is enabled
				MimeType:     webrtc.MimeTypeVP8,
				ClockRate:    90000,
				RTCPFeedback: DefaultVideoRTCPFeedback(),
			},
			{
				Name:         CodecVP9,
				Priority:     1, // Highest priority for SVC
				MimeType:     webrtc.MimeTypeVP9,
				ClockRate:    90000,
				RTCPFeedback: DefaultVideoRTCPFeedback(),
				SVCConfig: &SVCCodecConfig{
					Enabled:         true,
					ScalabilityMode: scalabilityMode,
				},
			},
			{
				Name:         CodecH264,
				Priority:     4,
				MimeType:     webrtc.MimeTypeH264,
				ClockRate:    90000,
				RTCPFeedback: DefaultVideoRTCPFeedback(),
				Profiles: []H264ProfileConfig{
					DefaultH264HighProfile(),
					DefaultH264BaselineProfile(),
				},
			},
			{
				Name:         CodecAV1,
				Priority:     2, // Second priority for SVC
				MimeType:     webrtc.MimeTypeAV1,
				ClockRate:    90000,
				RTCPFeedback: DefaultVideoRTCPFeedback(),
				SVCConfig: &SVCCodecConfig{
					Enabled:         true,
					ScalabilityMode: scalabilityMode,
				},
			},
		},
		AudioCodecs: []AudioCodecConfig{
			{
				Name:         CodecOpus,
				Priority:     1,
				MimeType:     webrtc.MimeTypeOpus,
				ClockRate:    48000,
				Channels:     2,
				RTCPFeedback: DefaultAudioRTCPFeedback(),
				FMTPParams:   DefaultOpusFMTP(),
			},
		},
	}
}

// IsSVCCodec returns true if the given codec name supports SVC.
func IsSVCCodec(name VideoCodecName) bool {
	return name == CodecVP9 || name == CodecAV1
}

// DefaultCodecConfigWithG711 returns the default codec configuration with G.711 support.
// G.711 is optional and provided for legacy compatibility.
func DefaultCodecConfigWithG711() CodecConfig {
	config := DefaultCodecConfig()
	config.AudioCodecs = append(config.AudioCodecs,
		AudioCodecConfig{
			Name:         CodecG711PCMU,
			Priority:     2,
			MimeType:     webrtc.MimeTypePCMU,
			ClockRate:    8000,
			Channels:     1,
			RTCPFeedback: DefaultAudioRTCPFeedback(),
		},
		AudioCodecConfig{
			Name:         CodecG711PCMA,
			Priority:     3,
			MimeType:     webrtc.MimeTypePCMA,
			ClockRate:    8000,
			Channels:     1,
			RTCPFeedback: DefaultAudioRTCPFeedback(),
		},
	)
	return config
}

// BuildFMTPLine builds the fmtp line for H.264 profile.
// Returns string like: "profile-level-id=640032;packetization-mode=1;level-asymmetry-allowed=1"
func (p H264ProfileConfig) BuildFMTPLine() string {
	return "profile-level-id=" + string(p.ProfileLevelID) +
		";packetization-mode=" + itoa(p.PacketizationMode) +
		";level-asymmetry-allowed=" + itoa(p.LevelAsymmetryAllowed)
}

// BuildFMTPLine builds the fmtp line for Opus codec.
// Returns string like: "minptime=10;useinbandfec=1;stereo=1"
func (p OpusFMTPParams) BuildFMTPLine() string {
	return "minptime=" + itoa(p.MinPTime) +
		";useinbandfec=" + itoa(p.UseInbandFEC) +
		";stereo=" + itoa(p.Stereo)
}

// itoa converts an integer to string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	negative := i < 0
	if negative {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// ValidateCodecConfig validates the codec configuration.
// Returns an error if the configuration is invalid.
func ValidateCodecConfig(config CodecConfig) error {
	if len(config.VideoCodecs) == 0 {
		return ErrNoVideoCodecs
	}
	if len(config.AudioCodecs) == 0 {
		return ErrNoAudioCodecs
	}

	// Validate video codecs
	for _, vc := range config.VideoCodecs {
		if err := validateVideoCodecConfig(vc); err != nil {
			return err
		}
	}

	// Validate audio codecs
	for _, ac := range config.AudioCodecs {
		if err := validateAudioCodecConfig(ac); err != nil {
			return err
		}
	}

	return nil
}

// validateVideoCodecConfig validates a single video codec configuration.
func validateVideoCodecConfig(vc VideoCodecConfig) error {
	switch vc.Name {
	case CodecVP8, CodecVP9, CodecAV1:
		// These codecs don't require profiles
		return nil
	case CodecH264:
		if len(vc.Profiles) == 0 {
			return ErrH264NoProfiles
		}
		return nil
	default:
		return ErrUnknownVideoCodec
	}
}

// validateAudioCodecConfig validates a single audio codec configuration.
func validateAudioCodecConfig(ac AudioCodecConfig) error {
	switch ac.Name {
	case CodecOpus, CodecG711PCMU, CodecG711PCMA:
		return nil
	default:
		return ErrUnknownAudioCodec
	}
}

// RegisterCodecs registers all configured codecs to the MediaEngine.
// This sets up the video and audio codecs with their RTCP feedback mechanisms.
//
// Important: This function registers codecs with explicit payload types
// to avoid conflicts. Audio payload types start at 111, video at 96.
//
// The function validates the configuration before registration and returns
// an error if any codec is invalid or misconfigured.
//
// Codecs are registered in priority order (lower priority value = higher precedence).
// The input config is not mutated; a sorted copy is used internally.
func RegisterCodecs(m *webrtc.MediaEngine, config CodecConfig) error {
	// Validate configuration first
	if err := ValidateCodecConfig(config); err != nil {
		return err
	}

	// Create a sorted copy to ensure registration order matches priority
	// without mutating the caller's config
	sortedConfig := config.Copy()
	sortedConfig.SortByPriority()

	// Track next available payload types
	var videoPayloadType webrtc.PayloadType = 96
	var audioPayloadType webrtc.PayloadType = 111

	// Register video codecs in priority order
	for _, vc := range sortedConfig.VideoCodecs {
		usedPT, err := registerVideoCodec(m, vc, videoPayloadType)
		if err != nil {
			return err
		}
		videoPayloadType = usedPT
	}

	// Register audio codecs in priority order
	for _, ac := range sortedConfig.AudioCodecs {
		usedPT, err := registerAudioCodec(m, ac, audioPayloadType)
		if err != nil {
			return err
		}
		audioPayloadType = usedPT
	}

	return nil
}

// registerVideoCodec registers a single video codec to the MediaEngine.
// Returns the next available payload type after registration.
func registerVideoCodec(m *webrtc.MediaEngine, vc VideoCodecConfig, nextPT webrtc.PayloadType) (webrtc.PayloadType, error) {
	feedback := ToWebRTCFeedbackSlice(vc.RTCPFeedback)

	switch vc.Name {
	case CodecH264:
		// Register H.264 with multiple profiles, each with a unique payload type
		for _, profile := range vc.Profiles {
			err := m.RegisterCodec(webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:     vc.MimeType,
					ClockRate:    vc.ClockRate,
					SDPFmtpLine:  profile.BuildFMTPLine(),
					RTCPFeedback: feedback,
				},
				PayloadType: nextPT,
			}, webrtc.RTPCodecTypeVideo)
			if err != nil {
				return nextPT, err
			}
			nextPT++
		}
	case CodecVP8, CodecVP9, CodecAV1:
		// Register VP8, VP9, AV1 without fmtp parameters
		err := m.RegisterCodec(webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     vc.MimeType,
				ClockRate:    vc.ClockRate,
				RTCPFeedback: feedback,
			},
			PayloadType: nextPT,
		}, webrtc.RTPCodecTypeVideo)
		if err != nil {
			return nextPT, err
		}
		nextPT++
	}

	return nextPT, nil
}

// registerAudioCodec registers a single audio codec to the MediaEngine.
// Returns the next available payload type after registration.
func registerAudioCodec(m *webrtc.MediaEngine, ac AudioCodecConfig, nextPT webrtc.PayloadType) (webrtc.PayloadType, error) {
	feedback := ToWebRTCFeedbackSlice(ac.RTCPFeedback)

	var fmtpLine string
	if ac.Name == CodecOpus {
		fmtpLine = ac.FMTPParams.BuildFMTPLine()
	}

	err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:     ac.MimeType,
			ClockRate:    ac.ClockRate,
			Channels:     ac.Channels,
			SDPFmtpLine:  fmtpLine,
			RTCPFeedback: feedback,
		},
		PayloadType: nextPT,
	}, webrtc.RTPCodecTypeAudio)

	if err != nil {
		return nextPT, err
	}
	return nextPT + 1, nil
}

// NewMediaEngineWithDefaults creates a new MediaEngine with default codec configuration.
func NewMediaEngineWithDefaults() (*webrtc.MediaEngine, error) {
	m := &webrtc.MediaEngine{}
	if err := RegisterCodecs(m, DefaultCodecConfig()); err != nil {
		return nil, err
	}
	return m, nil
}

// NewMediaEngineWithConfig creates a new MediaEngine with the given codec configuration.
func NewMediaEngineWithConfig(config CodecConfig) (*webrtc.MediaEngine, error) {
	m := &webrtc.MediaEngine{}
	if err := RegisterCodecs(m, config); err != nil {
		return nil, err
	}
	return m, nil
}

// Copy creates a deep copy of the CodecConfig.
// This is useful when you need to modify the config without affecting the original.
func (c CodecConfig) Copy() CodecConfig {
	videoCopy := make([]VideoCodecConfig, len(c.VideoCodecs))
	for i, vc := range c.VideoCodecs {
		videoCopy[i] = vc.Copy()
	}

	audioCopy := make([]AudioCodecConfig, len(c.AudioCodecs))
	for i, ac := range c.AudioCodecs {
		audioCopy[i] = ac.Copy()
	}

	return CodecConfig{
		VideoCodecs: videoCopy,
		AudioCodecs: audioCopy,
	}
}

// Copy creates a copy of the VideoCodecConfig.
func (vc VideoCodecConfig) Copy() VideoCodecConfig {
	feedbackCopy := make([]RTCPFeedback, len(vc.RTCPFeedback))
	copy(feedbackCopy, vc.RTCPFeedback)

	profilesCopy := make([]H264ProfileConfig, len(vc.Profiles))
	copy(profilesCopy, vc.Profiles)

	var svcConfigCopy *SVCCodecConfig
	if vc.SVCConfig != nil {
		svcConfigCopy = &SVCCodecConfig{
			Enabled:         vc.SVCConfig.Enabled,
			ScalabilityMode: vc.SVCConfig.ScalabilityMode,
		}
	}

	return VideoCodecConfig{
		Name:         vc.Name,
		Priority:     vc.Priority,
		MimeType:     vc.MimeType,
		ClockRate:    vc.ClockRate,
		RTCPFeedback: feedbackCopy,
		Profiles:     profilesCopy,
		SVCConfig:    svcConfigCopy,
	}
}

// Copy creates a copy of the AudioCodecConfig.
func (ac AudioCodecConfig) Copy() AudioCodecConfig {
	feedbackCopy := make([]RTCPFeedback, len(ac.RTCPFeedback))
	copy(feedbackCopy, ac.RTCPFeedback)

	return AudioCodecConfig{
		Name:         ac.Name,
		Priority:     ac.Priority,
		MimeType:     ac.MimeType,
		ClockRate:    ac.ClockRate,
		Channels:     ac.Channels,
		RTCPFeedback: feedbackCopy,
		FMTPParams:   ac.FMTPParams,
	}
}

// GetVideoCodecNames returns the list of video codec names from the configuration.
func (c CodecConfig) GetVideoCodecNames() []VideoCodecName {
	names := make([]VideoCodecName, len(c.VideoCodecs))
	for i, vc := range c.VideoCodecs {
		names[i] = vc.Name
	}
	return names
}

// GetAudioCodecNames returns the list of audio codec names from the configuration.
func (c CodecConfig) GetAudioCodecNames() []AudioCodecName {
	names := make([]AudioCodecName, len(c.AudioCodecs))
	for i, ac := range c.AudioCodecs {
		names[i] = ac.Name
	}
	return names
}

// HasVideoCodec checks if the configuration contains a specific video codec.
func (c CodecConfig) HasVideoCodec(name VideoCodecName) bool {
	for _, vc := range c.VideoCodecs {
		if vc.Name == name {
			return true
		}
	}
	return false
}

// HasAudioCodec checks if the configuration contains a specific audio codec.
func (c CodecConfig) HasAudioCodec(name AudioCodecName) bool {
	for _, ac := range c.AudioCodecs {
		if ac.Name == name {
			return true
		}
	}
	return false
}

// GetVideoCodecByName returns the video codec configuration by name.
func (c CodecConfig) GetVideoCodecByName(name VideoCodecName) *VideoCodecConfig {
	for i := range c.VideoCodecs {
		if c.VideoCodecs[i].Name == name {
			return &c.VideoCodecs[i]
		}
	}
	return nil
}

// GetAudioCodecByName returns the audio codec configuration by name.
func (c CodecConfig) GetAudioCodecByName(name AudioCodecName) *AudioCodecConfig {
	for i := range c.AudioCodecs {
		if c.AudioCodecs[i].Name == name {
			return &c.AudioCodecs[i]
		}
	}
	return nil
}

// SetVideoCodecPriority updates the priority of a video codec.
// Lower priority values mean higher priority.
func (c *CodecConfig) SetVideoCodecPriority(name VideoCodecName, priority int) {
	for i := range c.VideoCodecs {
		if c.VideoCodecs[i].Name == name {
			c.VideoCodecs[i].Priority = priority
			break
		}
	}
	c.sortVideoCodecsByPriority()
}

// SetAudioCodecPriority updates the priority of an audio codec.
// Lower priority values mean higher priority.
func (c *CodecConfig) SetAudioCodecPriority(name AudioCodecName, priority int) {
	for i := range c.AudioCodecs {
		if c.AudioCodecs[i].Name == name {
			c.AudioCodecs[i].Priority = priority
			break
		}
	}
	c.sortAudioCodecsByPriority()
}

// sortVideoCodecsByPriority sorts video codecs by priority (ascending).
func (c *CodecConfig) sortVideoCodecsByPriority() {
	// Simple insertion sort for small slices
	for i := 1; i < len(c.VideoCodecs); i++ {
		key := c.VideoCodecs[i]
		j := i - 1
		for j >= 0 && c.VideoCodecs[j].Priority > key.Priority {
			c.VideoCodecs[j+1] = c.VideoCodecs[j]
			j--
		}
		c.VideoCodecs[j+1] = key
	}
}

// sortAudioCodecsByPriority sorts audio codecs by priority (ascending).
func (c *CodecConfig) sortAudioCodecsByPriority() {
	// Simple insertion sort for small slices
	for i := 1; i < len(c.AudioCodecs); i++ {
		key := c.AudioCodecs[i]
		j := i - 1
		for j >= 0 && c.AudioCodecs[j].Priority > key.Priority {
			c.AudioCodecs[j+1] = c.AudioCodecs[j]
			j--
		}
		c.AudioCodecs[j+1] = key
	}
}

// SortByPriority sorts both video and audio codecs by their priority.
// This ensures registration order matches priority values.
func (c *CodecConfig) SortByPriority() {
	c.sortVideoCodecsByPriority()
	c.sortAudioCodecsByPriority()
}

// AddVideoCodec adds a video codec to the configuration.
// The codec is inserted according to its priority.
func (c *CodecConfig) AddVideoCodec(vc VideoCodecConfig) {
	c.VideoCodecs = append(c.VideoCodecs, vc)
	c.sortVideoCodecsByPriority()
}

// RemoveVideoCodec removes a video codec from the configuration by name.
func (c *CodecConfig) RemoveVideoCodec(name VideoCodecName) {
	var filtered []VideoCodecConfig
	for _, vc := range c.VideoCodecs {
		if vc.Name != name {
			filtered = append(filtered, vc)
		}
	}
	c.VideoCodecs = filtered
}

// AddAudioCodec adds an audio codec to the configuration.
// The codec is inserted according to its priority.
func (c *CodecConfig) AddAudioCodec(ac AudioCodecConfig) {
	c.AudioCodecs = append(c.AudioCodecs, ac)
	c.sortAudioCodecsByPriority()
}

// RemoveAudioCodec removes an audio codec from the configuration by name.
func (c *CodecConfig) RemoveAudioCodec(name AudioCodecName) {
	var filtered []AudioCodecConfig
	for _, ac := range c.AudioCodecs {
		if ac.Name != name {
			filtered = append(filtered, ac)
		}
	}
	c.AudioCodecs = filtered
}

package audiochannel

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"
)

// AudioProcessor provides utilities for processing audio samples
type AudioProcessor struct {
	sampleRate uint32
	channels   uint8
	bitDepth   uint8
}

// NewAudioProcessor creates a new audio processor
func NewAudioProcessor(sampleRate uint32, channels uint8, bitDepth uint8) *AudioProcessor {
	return &AudioProcessor{
		sampleRate: sampleRate,
		channels:   channels,
		bitDepth:   bitDepth,
	}
}

// PCMToSample converts PCM audio data to a media.Sample
func (ap *AudioProcessor) PCMToSample(pcmData []byte, timestamp time.Duration) *media.Sample {
	return &media.Sample{
		Data:            pcmData,
		Duration:        ap.calculateDuration(len(pcmData)),
		PacketTimestamp: uint32(timestamp.Nanoseconds() / 1000000), // Convert to milliseconds
	}
}

// SampleToPCM extracts PCM data from a media.Sample
func (ap *AudioProcessor) SampleToPCM(sample *media.Sample) []byte {
	return sample.Data
}

// calculateDuration calculates the duration of audio data based on sample rate and channels
func (ap *AudioProcessor) calculateDuration(dataLength int) time.Duration {
	bytesPerSample := int(ap.bitDepth / 8)
	samplesPerChannel := dataLength / (int(ap.channels) * bytesPerSample)
	durationMs := float64(samplesPerChannel) / float64(ap.sampleRate) * 1000
	return time.Duration(durationMs) * time.Millisecond
}

// GenerateSilence generates silent audio data
func (ap *AudioProcessor) GenerateSilence(duration time.Duration) *media.Sample {
	samplesPerChannel := int(duration.Seconds() * float64(ap.sampleRate))
	totalBytes := samplesPerChannel * int(ap.channels) * int(ap.bitDepth/8)

	silentData := make([]byte, totalBytes)
	// PCM silence is all zeros, so no need to fill

	return &media.Sample{
		Data:     silentData,
		Duration: duration,
	}
}

// GenerateTone generates a sine wave tone
func (ap *AudioProcessor) GenerateTone(frequency float64, duration time.Duration, amplitude float64) *media.Sample {
	if amplitude > 1.0 {
		amplitude = 1.0
	}
	if amplitude < 0.0 {
		amplitude = 0.0
	}

	samplesPerChannel := int(duration.Seconds() * float64(ap.sampleRate))
	totalSamples := samplesPerChannel * int(ap.channels)

	var data []byte

	switch ap.bitDepth {
	case 16:
		data = make([]byte, totalSamples*2)
		for i := 0; i < samplesPerChannel; i++ {
			// Generate sine wave sample
			t := float64(i) / float64(ap.sampleRate)
			sample := math.Sin(2*math.Pi*frequency*t) * amplitude

			// Convert to 16-bit PCM
			pcmSample := int16(sample * 32767)

			// Write to all channels
			for ch := 0; ch < int(ap.channels); ch++ {
				offset := (i*int(ap.channels) + ch) * 2
				binary.LittleEndian.PutUint16(data[offset:offset+2], uint16(pcmSample))
			}
		}
	case 8:
		data = make([]byte, totalSamples)
		for i := 0; i < samplesPerChannel; i++ {
			t := float64(i) / float64(ap.sampleRate)
			sample := math.Sin(2*math.Pi*frequency*t) * amplitude

			// Convert to 8-bit PCM (unsigned)
			pcmSample := uint8((sample + 1.0) * 127.5)

			// Write to all channels
			for ch := 0; ch < int(ap.channels); ch++ {
				offset := i*int(ap.channels) + ch
				data[offset] = pcmSample
			}
		}
	default:
		// Default to 16-bit
		return ap.GenerateTone(frequency, duration, amplitude)
	}

	return &media.Sample{
		Data:     data,
		Duration: duration,
	}
}

// MixSamples mixes multiple audio samples together
func (ap *AudioProcessor) MixSamples(samples ...*media.Sample) (*media.Sample, error) {
	if len(samples) == 0 {
		return nil, fmt.Errorf("no samples to mix")
	}

	if len(samples) == 1 {
		return samples[0], nil
	}

	// Find the longest sample to determine output length
	maxLength := 0
	var longestDuration time.Duration

	for _, sample := range samples {
		if len(sample.Data) > maxLength {
			maxLength = len(sample.Data)
			longestDuration = sample.Duration
		}
	}

	mixedData := make([]int32, maxLength/2) // Assuming 16-bit samples

	// Mix all samples
	for _, sample := range samples {
		sampleData := sample.Data
		for i := 0; i < len(sampleData)-1; i += 2 {
			if i/2 < len(mixedData) {
				// Convert from little-endian 16-bit to int32 for mixing
				sample16 := int32(int16(binary.LittleEndian.Uint16(sampleData[i : i+2])))
				mixedData[i/2] += sample16
			}
		}
	}

	// Convert back to 16-bit and apply normalization if needed
	outputData := make([]byte, maxLength)
	for i, mixed := range mixedData {
		// Simple clipping to prevent overflow
		if mixed > 32767 {
			mixed = 32767
		} else if mixed < -32768 {
			mixed = -32768
		}

		binary.LittleEndian.PutUint16(outputData[i*2:i*2+2], uint16(mixed))
	}

	return &media.Sample{
		Data:     outputData,
		Duration: longestDuration,
	}, nil
}

// AnalyzeSample provides basic audio analysis
func (ap *AudioProcessor) AnalyzeSample(sample *media.Sample) AudioAnalysis {
	analysis := AudioAnalysis{
		Duration:   sample.Duration,
		DataLength: len(sample.Data),
		Channels:   ap.channels,
		SampleRate: ap.sampleRate,
		BitDepth:   ap.bitDepth,
	}

	if len(sample.Data) == 0 {
		return analysis
	}

	// Calculate peak amplitude and RMS for 16-bit audio
	if ap.bitDepth == 16 {
		var sum int64
		var peak int16
		sampleCount := len(sample.Data) / 2

		for i := 0; i < len(sample.Data)-1; i += 2 {
			sample16 := int16(binary.LittleEndian.Uint16(sample.Data[i : i+2]))

			// Calculate peak
			abs := sample16
			if abs < 0 {
				abs = -abs
			}
			if abs > peak {
				peak = abs
			}

			// Calculate sum for RMS
			sum += int64(sample16) * int64(sample16)
		}

		analysis.PeakAmplitude = float64(peak) / 32768.0
		if sampleCount > 0 {
			analysis.RMSAmplitude = math.Sqrt(float64(sum)/float64(sampleCount)) / 32768.0
		}
	}

	return analysis
}

// AudioAnalysis contains analysis results for an audio sample
type AudioAnalysis struct {
	Duration      time.Duration `json:"duration"`
	DataLength    int           `json:"data_length"`
	Channels      uint8         `json:"channels"`
	SampleRate    uint32        `json:"sample_rate"`
	BitDepth      uint8         `json:"bit_depth"`
	PeakAmplitude float64       `json:"peak_amplitude"`
	RMSAmplitude  float64       `json:"rms_amplitude"`
}

package audiochannel

import (
	"context"
	"log"
	"time"

	pkgwebrtc "github.com/HMasataka/choice/pkg/webrtc"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// NewAudioChannelExample creates a new audio channel with example handlers
// This function demonstrates how to use the AudioChannel similar to datachannel.NewDataChannel
func NewAudioChannelExample(ctx context.Context, pc *pkgwebrtc.PeerConnection) (*AudioChannel, error) {
	// Use default Opus configuration
	config := DefaultOpusConfig()
	config.Label = "example-audio"

	// Create audio channel
	ac, err := NewAudioChannel(ctx, config, pc)
	if err != nil {
		log.Printf("Failed to create audio channel: %v", err)
		return nil, err
	}

	// Create audio processor for sample processing
	processor := NewAudioProcessor(config.SampleRate, config.Channels, 16)

	// Set up event handlers
	ac.OnTrackReceived(func(track *webrtc.TrackRemote) {
		log.Printf("Audio track received: %s, codec: %s", track.ID(), track.Codec().MimeType)
	})

	ac.OnSample(func(sample *media.Sample) {
		// Analyze the received audio sample
		analysis := processor.AnalyzeSample(sample)
		log.Printf("Received audio sample - Duration: %v, Peak: %.2f, RMS: %.2f",
			analysis.Duration, analysis.PeakAmplitude, analysis.RMSAmplitude)

		// Example: Echo the received audio back (for testing purposes)
		// In a real application, you might process the audio differently
		if err := ac.SendSample(sample); err != nil {
			log.Printf("Failed to echo audio sample: %v", err)
		}
	})

	ac.OnOpen(func() {
		log.Println("Audio channel opened")

		// Example: Send a test tone when the channel opens
		go func() {
			// Generate a 440Hz tone (A4 note) for 1 second
			testTone := processor.GenerateTone(440.0, time.Second, 0.5)

			if err := ac.SendSample(testTone); err != nil {
				log.Printf("Failed to send test tone: %v", err)
			} else {
				log.Println("Test tone sent successfully")
			}
		}()
	})

	ac.OnClose(func() {
		log.Println("Audio channel closed")
	})

	ac.OnError(func(err error) {
		log.Printf("Audio channel error: %v", err)
	})

	return ac, nil
}

// AudioStreamExample demonstrates continuous audio streaming
func AudioStreamExample(ctx context.Context, ac *AudioChannel) {
	processor := NewAudioProcessor(48000, 2, 16)

	// Create a ticker for sending audio at regular intervals
	ticker := time.NewTicker(20 * time.Millisecond) // 20ms intervals for 50fps audio
	defer ticker.Stop()

	frequency := 440.0 // Start with A4

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Audio streaming panic recovered: %v", r)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				log.Println("Audio streaming stopped by context")
				return
			case <-ticker.C:
				// Generate a short audio segment
				sample := processor.GenerateTone(frequency, 20*time.Millisecond, 0.3)

				if err := ac.SendSample(sample); err != nil {
					log.Printf("Failed to send audio sample: %v", err)
					continue
				}

				// Gradually change frequency for a sweep effect
				frequency += 1.0
				if frequency > 880.0 { // Go up to A5
					frequency = 440.0 // Reset to A4
				}
			}
		}
	}()

	log.Println("Audio streaming started")
}

// AudioFileExample demonstrates how to handle audio file data
func AudioFileExample(ac *AudioChannel, audioData []byte, sampleRate uint32, channels uint8) error {
	processor := NewAudioProcessor(sampleRate, channels, 16)

	// Convert raw audio data to sample
	sample := processor.PCMToSample(audioData, time.Duration(0))

	// Analyze the audio
	analysis := processor.AnalyzeSample(sample)
	log.Printf("Audio file analysis - Duration: %v, Peak: %.2f, RMS: %.2f",
		analysis.Duration, analysis.PeakAmplitude, analysis.RMSAmplitude)

	// Send the audio sample
	return ac.SendSample(sample)
}

// AudioMixerExample demonstrates mixing multiple audio sources
func AudioMixerExample(ac *AudioChannel, processor *AudioProcessor) {
	// Generate different frequency tones
	tone1 := processor.GenerateTone(440.0, time.Second, 0.3) // A4
	tone2 := processor.GenerateTone(554.37, time.Second, 0.3) // C#5
	tone3 := processor.GenerateTone(659.25, time.Second, 0.3) // E5

	// Mix the tones together to create a chord
	mixedSample, err := processor.MixSamples(tone1, tone2, tone3)
	if err != nil {
		log.Printf("Failed to mix audio samples: %v", err)
		return
	}

	// Send the mixed audio
	if err := ac.SendSample(mixedSample); err != nil {
		log.Printf("Failed to send mixed audio: %v", err)
	} else {
		log.Println("Mixed audio chord sent successfully")
	}
}

// GetAudioChannelStats demonstrates how to retrieve and log statistics
func GetAudioChannelStats(ac *AudioChannel) {
	stats := ac.GetStats()

	log.Printf("Audio Channel Statistics:")
	log.Printf("  Label: %s", stats.Label)
	log.Printf("  Codec: %s", stats.Codec)
	log.Printf("  Sample Rate: %d Hz", stats.SampleRate)
	log.Printf("  Channels: %d", stats.Channels)
	log.Printf("  Packets Sent: %d", stats.PacketsSent)
	log.Printf("  Packets Received: %d", stats.PacketsReceived)
	log.Printf("  Bytes Sent: %d", stats.BytesSent)
	log.Printf("  Bytes Received: %d", stats.BytesReceived)
	log.Printf("  Closed: %t", stats.Closed)
}
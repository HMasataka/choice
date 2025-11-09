package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/HMasataka/choice/pkg/audio"
)

func main() {
	fmt.Println("Microphone Test Example")

	// Create microphone recorder
	// Parameters: sampleRate=44100, channels=1 (mono), framesPerBuffer=1024
	recorder, err := audio.NewMicRecorder(44100, 1, 1024, func(samples []int16) {
		// Calculate RMS (Root Mean Square) for volume level
		var sum float64
		for _, sample := range samples {
			sum += float64(sample) * float64(sample)
		}
		rms := sum / float64(len(samples))
		volume := int(rms / 1000000) // Simple volume indicator

		// Print volume bar
		volumeBar := ""
		for i := 0; i < volume && i < 20; i++ {
			volumeBar += "█"
		}
		fmt.Printf("\rVolume: %-20s", volumeBar)
	})

	if err != nil {
		log.Fatalf("Failed to create microphone recorder: %v", err)
	}
	defer recorder.Close()

	fmt.Printf("Sample Rate: %d Hz\n", recorder.SampleRate())
	fmt.Printf("Channels: %d\n", recorder.NumChannels())
	fmt.Printf("Frames per Buffer: %d\n", recorder.FramesPerBuffer())

	// Test 1: Real-time monitoring
	fmt.Println("\nTest 1: Real-time audio monitoring (5 seconds)")
	fmt.Println("Speak into your microphone...")

	if err := recorder.Start(); err != nil {
		log.Fatalf("Failed to start recording: %v", err)
	}

	time.Sleep(5 * time.Second)

	if err := recorder.Stop(); err != nil {
		log.Fatalf("Failed to stop recording: %v", err)
	}

	fmt.Println("\nReal-time monitoring stopped.")

	// Test 2: Record to buffer
	fmt.Println("\nTest 2: Recording to buffer (3 seconds)")
	fmt.Println("Speak into your microphone...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	samples, err := recorder.RecordToBuffer(ctx, 3*time.Second)
	if err != nil {
		log.Fatalf("Failed to record to buffer: %v", err)
	}

	fmt.Printf("Recorded %d samples\n", len(samples))

	// Calculate and display basic statistics
	if len(samples) > 0 {
		var min, max int16 = samples[0], samples[0]
		var sum int64

		for _, sample := range samples {
			if sample < min {
				min = sample
			}
			if sample > max {
				max = sample
			}
			sum += int64(sample)
		}

		avg := sum / int64(len(samples))

		fmt.Printf("Sample statistics:\n")
		fmt.Printf("  Min: %d\n", min)
		fmt.Printf("  Max: %d\n", max)
		fmt.Printf("  Average: %d\n", avg)
		fmt.Printf("  Duration: %.2f seconds\n", float64(len(samples))/float64(recorder.SampleRate()))
	}

	fmt.Println("\nMicrophone test completed successfully!")
}
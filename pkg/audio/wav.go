package audio

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
)

// AudioCallback is called when audio data is available
type AudioCallback func(samples []int16)

// MicRecorder records audio from microphone in real-time
type MicRecorder struct {
	stream       *portaudio.Stream
	sampleRate   int
	numChannels  int
	framesPerBuffer int
	callback     AudioCallback
	mu           sync.RWMutex
	isRecording  bool
}

// WAVReader reads WAV file samples
type WAVReader struct {
	file          *os.File
	sampleRate    uint32
	numChannels   uint16
	bitsPerSample uint16
	dataSize      uint32
	dataOffset    int64
}

// WAVHeader represents WAV file header
type WAVHeader struct {
	ChunkID       [4]byte
	ChunkSize     uint32
	Format        [4]byte
	Subchunk1ID   [4]byte
	Subchunk1Size uint32
	AudioFormat   uint16
	NumChannels   uint16
	SampleRate    uint32
	ByteRate      uint32
	BlockAlign    uint16
	BitsPerSample uint16
	Subchunk2ID   [4]byte
	Subchunk2Size uint32
}

// NewWAVReader creates a new WAV file reader
func NewWAVReader(filename string) (*WAVReader, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	var header WAVHeader
	if err := binary.Read(file, binary.LittleEndian, &header); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Validate WAV format
	if string(header.ChunkID[:]) != "RIFF" || string(header.Format[:]) != "WAVE" {
		file.Close()
		return nil, fmt.Errorf("invalid WAV file format")
	}

	if header.AudioFormat != 1 {
		file.Close()
		return nil, fmt.Errorf("only PCM format is supported")
	}

	// Get current position (data offset)
	dataOffset, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to get data offset: %w", err)
	}

	return &WAVReader{
		file:          file,
		sampleRate:    header.SampleRate,
		numChannels:   header.NumChannels,
		bitsPerSample: header.BitsPerSample,
		dataSize:      header.Subchunk2Size,
		dataOffset:    dataOffset,
	}, nil
}

// SampleRate returns the sample rate
func (r *WAVReader) SampleRate() uint32 {
	return r.sampleRate
}

// NumChannels returns the number of channels
func (r *WAVReader) NumChannels() uint16 {
	return r.numChannels
}

// ReadSamples reads samples from WAV file
// Returns samples as int16 slice (converting from 8/24/32 bit if necessary)
func (r *WAVReader) ReadSamples(numSamples int) ([]int16, error) {
	samples := make([]int16, numSamples*int(r.numChannels))

	switch r.bitsPerSample {
	case 16:
		// Direct read for 16-bit samples
		if err := binary.Read(r.file, binary.LittleEndian, samples); err != nil {
			if err == io.EOF {
				return samples[:0], io.EOF
			}
			return nil, fmt.Errorf("failed to read samples: %w", err)
		}
	case 8:
		// Convert 8-bit to 16-bit
		buf := make([]uint8, len(samples))
		if err := binary.Read(r.file, binary.LittleEndian, buf); err != nil {
			if err == io.EOF {
				return samples[:0], io.EOF
			}
			return nil, fmt.Errorf("failed to read samples: %w", err)
		}
		for i, v := range buf {
			// Convert unsigned 8-bit to signed 16-bit
			samples[i] = int16(v-128) << 8
		}
	default:
		return nil, fmt.Errorf("unsupported bit depth: %d", r.bitsPerSample)
	}

	return samples, nil
}

// Reset resets the reader to the beginning of audio data
func (r *WAVReader) Reset() error {
	_, err := r.file.Seek(r.dataOffset, io.SeekStart)
	return err
}

// Close closes the WAV file
func (r *WAVReader) Close() error {
	return r.file.Close()
}

// NewMicRecorder creates a new microphone recorder
func NewMicRecorder(sampleRate, numChannels, framesPerBuffer int, callback AudioCallback) (*MicRecorder, error) {
	if err := portaudio.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize PortAudio: %w", err)
	}

	recorder := &MicRecorder{
		sampleRate:      sampleRate,
		numChannels:     numChannels,
		framesPerBuffer: framesPerBuffer,
		callback:        callback,
	}

	// Get default input device
	device, err := portaudio.DefaultInputDevice()
	if err != nil {
		portaudio.Terminate()
		return nil, fmt.Errorf("failed to get default input device: %w", err)
	}

	// Configure stream parameters
	inputParameters := portaudio.StreamParameters{
		Input: portaudio.StreamDeviceParameters{
			Device:   device,
			Channels: numChannels,
			Latency:  device.DefaultLowInputLatency,
		},
		SampleRate:      float64(sampleRate),
		FramesPerBuffer: framesPerBuffer,
	}

	// Create buffer for audio data
	buffer := make([]int16, framesPerBuffer*numChannels)

	// Create stream with callback
	stream, err := portaudio.OpenStream(inputParameters, func(in []int16) {
		// Copy input data to avoid data race
		copy(buffer, in)

		// Call user callback if recording
		recorder.mu.RLock()
		if recorder.isRecording && recorder.callback != nil {
			recorder.callback(buffer[:len(in)])
		}
		recorder.mu.RUnlock()
	})

	if err != nil {
		portaudio.Terminate()
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}

	recorder.stream = stream
	return recorder, nil
}

// Start starts recording from the microphone
func (r *MicRecorder) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isRecording {
		return nil // Already recording
	}

	if err := r.stream.Start(); err != nil {
		return fmt.Errorf("failed to start stream: %w", err)
	}

	r.isRecording = true
	return nil
}

// Stop stops recording from the microphone
func (r *MicRecorder) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isRecording {
		return nil // Already stopped
	}

	if err := r.stream.Stop(); err != nil {
		return fmt.Errorf("failed to stop stream: %w", err)
	}

	r.isRecording = false
	return nil
}

// IsRecording returns true if currently recording
func (r *MicRecorder) IsRecording() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.isRecording
}

// SetCallback sets the audio callback function
func (r *MicRecorder) SetCallback(callback AudioCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callback = callback
}

// SampleRate returns the sample rate
func (r *MicRecorder) SampleRate() int {
	return r.sampleRate
}

// NumChannels returns the number of channels
func (r *MicRecorder) NumChannels() int {
	return r.numChannels
}

// FramesPerBuffer returns the frames per buffer
func (r *MicRecorder) FramesPerBuffer() int {
	return r.framesPerBuffer
}

// Close closes the microphone recorder and releases resources
func (r *MicRecorder) Close() error {
	r.Stop() // Stop recording if running

	if r.stream != nil {
		if err := r.stream.Close(); err != nil {
			portaudio.Terminate()
			return fmt.Errorf("failed to close stream: %w", err)
		}
	}

	portaudio.Terminate()
	return nil
}

// RecordToBuffer records audio for the specified duration and returns the samples
func (r *MicRecorder) RecordToBuffer(ctx context.Context, duration time.Duration) ([]int16, error) {
	var samples []int16
	var mu sync.Mutex

	// Set callback to collect samples
	originalCallback := r.callback
	r.SetCallback(func(audioSamples []int16) {
		mu.Lock()
		samples = append(samples, audioSamples...)
		mu.Unlock()
	})

	// Restore original callback when done
	defer r.SetCallback(originalCallback)

	// Start recording
	if err := r.Start(); err != nil {
		return nil, fmt.Errorf("failed to start recording: %w", err)
	}

	// Wait for duration or context cancellation
	select {
	case <-time.After(duration):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Stop recording
	if err := r.Stop(); err != nil {
		return nil, fmt.Errorf("failed to stop recording: %w", err)
	}

	mu.Lock()
	result := make([]int16, len(samples))
	copy(result, samples)
	mu.Unlock()

	return result, nil
}

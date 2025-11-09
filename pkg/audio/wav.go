package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

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

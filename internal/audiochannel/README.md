# AudioChannel Package

This package provides WebRTC audio channel functionality, inspired by the `datachannel` package implementation. It allows for sending and receiving audio data through WebRTC peer connections with built-in audio processing capabilities.

## Features

- **Audio Channel Management**: Create and manage WebRTC audio channels
- **Audio Processing**: Built-in audio sample processing, mixing, and generation
- **Event Handling**: Comprehensive event system for audio track events
- **Statistics**: Real-time audio channel statistics tracking
- **Codec Support**: Opus codec support with configurable parameters

## Usage

### Basic Audio Channel Creation

```go
package main

import (
    "context"
    "log"

    "github.com/HMasataka/choice/internal/audiochannel"
    pkgwebrtc "github.com/HMasataka/choice/pkg/webrtc"
)

func main() {
    ctx := context.Background()

    // Create peer connection
    options := pkgwebrtc.DefaultPeerConnectionOptions()
    pc, err := pkgwebrtc.NewPeerConnection(ctx, "audio-peer", options)
    if err != nil {
        log.Fatal(err)
    }
    defer pc.Close()

    // Create audio channel with default Opus configuration
    config := audiochannel.DefaultOpusConfig()
    ac, err := audiochannel.NewAudioChannel(ctx, config, pc)
    if err != nil {
        log.Fatal(err)
    }
    defer ac.Close()

    // Set up event handlers
    ac.OnTrackReceived(func(track *webrtc.TrackRemote) {
        log.Printf("Audio track received: %s", track.ID())
    })

    ac.OnSample(func(sample *media.Sample) {
        log.Printf("Received audio sample: %d bytes", len(sample.Data))
    })
}
```

### Using the Example Helper

```go
// Create an audio channel with pre-configured example handlers
ac, err := audiochannel.NewAudioChannelExample(ctx, pc)
if err != nil {
    log.Fatal(err)
}
defer ac.Close()
```

### Audio Processing

```go
// Create audio processor
processor := audiochannel.NewAudioProcessor(48000, 2, 16)

// Generate a test tone
testTone := processor.GenerateTone(440.0, time.Second, 0.5) // A4 note
err := ac.SendSample(testTone)
if err != nil {
    log.Printf("Failed to send audio: %v", err)
}

// Generate silence
silence := processor.GenerateSilence(500 * time.Millisecond)
err = ac.SendSample(silence)

// Mix multiple audio samples
tone1 := processor.GenerateTone(440.0, time.Second, 0.3) // A4
tone2 := processor.GenerateTone(554.37, time.Second, 0.3) // C#5
mixedSample, err := processor.MixSamples(tone1, tone2)
if err == nil {
    ac.SendSample(mixedSample)
}
```

### Audio Streaming

```go
// Start continuous audio streaming
audiochannel.AudioStreamExample(ctx, ac)
```

### Statistics

```go
// Get audio channel statistics
stats := ac.GetStats()
log.Printf("Packets sent: %d, received: %d", stats.PacketsSent, stats.PacketsReceived)
log.Printf("Bytes sent: %d, received: %d", stats.BytesSent, stats.BytesReceived)
```

## Configuration

### AudioChannelConfig

```go
config := audiochannel.AudioChannelConfig{
    Label:      "custom-audio",
    Codec:      pkgwebrtc.GetOpusCodec(),
    SampleRate: 48000,
    Channels:   2,
}
```

### Custom Codec Configuration

```go
customCodec := webrtc.RTPCodecCapability{
    MimeType:     webrtc.MimeTypeOpus,
    ClockRate:    48000,
    Channels:     1, // Mono
    SDPFmtpLine:  "minptime=10;useinbandfec=1",
}

config := audiochannel.AudioChannelConfig{
    Label:      "mono-audio",
    Codec:      customCodec,
    SampleRate: 48000,
    Channels:   1,
}
```

## API Reference

### AudioChannel

- `NewAudioChannel(ctx, config, pc)` - Create new audio channel
- `SendSample(sample)` - Send audio sample
- `GetStats()` - Get channel statistics
- `Close()` - Close channel and cleanup resources

### Event Handlers

- `OnTrackReceived(handler)` - Called when remote track is received
- `OnSample(handler)` - Called when audio sample is received
- `OnOpen(handler)` - Called when channel opens
- `OnClose(handler)` - Called when channel closes
- `OnError(handler)` - Called on errors

### AudioProcessor

- `NewAudioProcessor(sampleRate, channels, bitDepth)` - Create processor
- `GenerateTone(frequency, duration, amplitude)` - Generate sine wave
- `GenerateSilence(duration)` - Generate silent audio
- `MixSamples(samples...)` - Mix multiple audio samples
- `AnalyzeSample(sample)` - Analyze audio sample

## Error Handling

The package defines several error types:

- `ErrAudioChannelClosed` - Channel is closed
- `ErrAudioChannelNotReady` - Channel not ready for operation
- `ErrPeerConnectionNotSet` - Peer connection not provided

## Thread Safety

The AudioChannel implementation is thread-safe and can be used concurrently from multiple goroutines. All operations are protected by appropriate synchronization mechanisms.

## Examples

See `example.go` for comprehensive usage examples including:

- Basic audio channel setup
- Audio streaming
- File audio handling
- Audio mixing
- Statistics monitoring

## Integration with DataChannel

This package follows the same design patterns as the `datachannel` package, making it easy to use both data and audio channels together in the same WebRTC application.
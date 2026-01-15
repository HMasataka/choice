package media

import (
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func TestTrackKind_String(t *testing.T) {
	tests := []struct {
		name string
		kind TrackKind
		want string
	}{
		{"video", TrackKindVideo, "video"},
		{"audio", TrackKindAudio, "audio"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.want {
				t.Errorf("TrackKind.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrackKind_Validate(t *testing.T) {
	tests := []struct {
		name    string
		kind    TrackKind
		wantErr bool
	}{
		{"valid video", TrackKindVideo, false},
		{"valid audio", TrackKindAudio, false},
		{"invalid", TrackKind("invalid"), true},
		{"empty", TrackKind(""), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.kind.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("TrackKind.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSimulcastLayer_String(t *testing.T) {
	tests := []struct {
		name  string
		layer SimulcastLayer
		want  string
	}{
		{"high", SimulcastLayerHigh, "h"},
		{"medium", SimulcastLayerMedium, "m"},
		{"low", SimulcastLayerLow, "l"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.layer.String(); got != tt.want {
				t.Errorf("SimulcastLayer.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSimulcastLayer_Validate(t *testing.T) {
	tests := []struct {
		name    string
		layer   SimulcastLayer
		wantErr bool
	}{
		{"valid high", SimulcastLayerHigh, false},
		{"valid medium", SimulcastLayerMedium, false},
		{"valid low", SimulcastLayerLow, false},
		{"invalid", SimulcastLayer("x"), true},
		{"empty", SimulcastLayer(""), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.layer.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("SimulcastLayer.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGenerateTrackID(t *testing.T) {
	// Generate multiple IDs and ensure they are unique
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := GenerateTrackID()
		if id == "" {
			t.Error("GenerateTrackID() returned empty string")
		}
		if ids[id.String()] {
			t.Errorf("GenerateTrackID() generated duplicate ID: %s", id)
		}
		ids[id.String()] = true
	}
}

func TestTrackID_Validate(t *testing.T) {
	tests := []struct {
		name    string
		id      TrackID
		wantErr bool
	}{
		{"valid ID", GenerateTrackID(), false},
		{"empty ID", TrackID(""), true},
		{"invalid UUID format", TrackID("not-a-uuid"), true},
		{"invalid UUID - too short", TrackID("12345"), true},
		{"valid UUID v4 format", TrackID("550e8400-e29b-41d4-a716-446655440000"), false},
		{"valid UUID v1 format - wrong version", TrackID("c56a4180-65aa-11ec-90d6-0242ac120003"), true}, // UUID v1
		{"valid UUID v5 format - wrong version", TrackID("74738ff5-5367-5958-9aee-98fffdcd1876"), true}, // UUID v5
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.id.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("TrackID.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTrackMetadata_Copy(t *testing.T) {
	original := &TrackMetadata{
		Label:     "camera",
		Simulcast: true,
		Layers:    []SimulcastLayer{SimulcastLayerHigh, SimulcastLayerMedium},
		MID:       "mid-1",
		SSRC:      12345,
		Custom:    map[string]interface{}{"key": "value"},
	}

	copied := original.Copy()

	// Verify values are equal
	if copied.Label != original.Label {
		t.Errorf("Copy() Label = %v, want %v", copied.Label, original.Label)
	}
	if copied.Simulcast != original.Simulcast {
		t.Errorf("Copy() Simulcast = %v, want %v", copied.Simulcast, original.Simulcast)
	}
	if copied.MID != original.MID {
		t.Errorf("Copy() MID = %v, want %v", copied.MID, original.MID)
	}
	if copied.SSRC != original.SSRC {
		t.Errorf("Copy() SSRC = %v, want %v", copied.SSRC, original.SSRC)
	}

	// Verify deep copy (modify original should not affect copy)
	original.Label = "modified"
	if copied.Label == "modified" {
		t.Error("Copy() did not create a deep copy of Label")
	}

	original.Layers[0] = SimulcastLayerLow
	if copied.Layers[0] == SimulcastLayerLow {
		t.Error("Copy() did not create a deep copy of Layers")
	}

	original.Custom["key"] = "modified"
	if copied.Custom["key"] == "modified" {
		t.Error("Copy() did not create a deep copy of Custom")
	}
}

func TestTrackMetadata_Copy_Nil(t *testing.T) {
	var metadata *TrackMetadata
	copied := metadata.Copy()
	if copied != nil {
		t.Error("Copy() of nil metadata should return nil")
	}
}

func TestNewLocalTrack(t *testing.T) {
	publisherID := "publisher-1"
	roomID := "room-1"
	kind := TrackKindVideo
	metadata := &TrackMetadata{
		Label:     "camera",
		Simulcast: true,
		Layers:    []SimulcastLayer{SimulcastLayerHigh, SimulcastLayerMedium},
	}

	// Create a mock webrtc.TrackRemote
	// Note: In real tests, you might need to use a more sophisticated mock
	track := &webrtc.TrackRemote{}

	localTrack := NewLocalTrack(publisherID, roomID, kind, track, metadata)

	if localTrack.ID == "" {
		t.Error("NewLocalTrack() ID is empty")
	}
	if localTrack.Kind != kind {
		t.Errorf("NewLocalTrack() Kind = %v, want %v", localTrack.Kind, kind)
	}
	if localTrack.PublisherID != publisherID {
		t.Errorf("NewLocalTrack() PublisherID = %v, want %v", localTrack.PublisherID, publisherID)
	}
	if localTrack.RoomID != roomID {
		t.Errorf("NewLocalTrack() RoomID = %v, want %v", localTrack.RoomID, roomID)
	}
	if localTrack.Track != track {
		t.Error("NewLocalTrack() Track is not set correctly")
	}
	if localTrack.metadata == nil {
		t.Error("NewLocalTrack() metadata is nil")
	}
	if localTrack.CreatedAt.IsZero() {
		t.Error("NewLocalTrack() CreatedAt is zero")
	}
	if localTrack.UpdatedAt.IsZero() {
		t.Error("NewLocalTrack() UpdatedAt is zero")
	}
}

func TestLocalTrack_GetMetadata(t *testing.T) {
	metadata := &TrackMetadata{
		Label:     "camera",
		Simulcast: true,
	}
	track := NewLocalTrack("pub-1", "room-1", TrackKindVideo, &webrtc.TrackRemote{}, metadata)

	got := track.GetMetadata()
	if got == nil {
		t.Fatal("GetMetadata() returned nil")
	}
	if got.Label != metadata.Label {
		t.Errorf("GetMetadata() Label = %v, want %v", got.Label, metadata.Label)
	}

	// Verify it returns a copy (modifying returned value should not affect original)
	got.Label = "modified"
	if track.metadata.Label == "modified" {
		t.Error("GetMetadata() did not return a copy")
	}
}

func TestLocalTrack_UpdateMetadata(t *testing.T) {
	track := NewLocalTrack("pub-1", "room-1", TrackKindVideo, &webrtc.TrackRemote{}, &TrackMetadata{
		Label: "original",
	})

	originalUpdatedAt := track.UpdatedAt
	time.Sleep(10 * time.Millisecond) // Ensure time difference

	newMetadata := &TrackMetadata{
		Label:     "updated",
		Simulcast: true,
	}
	track.UpdateMetadata(newMetadata)

	got := track.GetMetadata()
	if got.Label != "updated" {
		t.Errorf("UpdateMetadata() Label = %v, want %v", got.Label, "updated")
	}
	if got.Simulcast != true {
		t.Error("UpdateMetadata() Simulcast not updated")
	}
	if !track.UpdatedAt.After(originalUpdatedAt) {
		t.Error("UpdateMetadata() did not update UpdatedAt timestamp")
	}

	// Verify it stores a copy (modifying input should not affect stored metadata)
	newMetadata.Label = "modified"
	if track.metadata.Label == "modified" {
		t.Error("UpdateMetadata() did not store a copy")
	}
}

func TestLocalTrack_GetSSRC(t *testing.T) {
	track := NewLocalTrack("pub-1", "room-1", TrackKindVideo, &webrtc.TrackRemote{}, &TrackMetadata{
		SSRC: 12345,
	})

	if got := track.GetSSRC(); got != 12345 {
		t.Errorf("GetSSRC() = %v, want %v", got, 12345)
	}

	// Test with nil metadata
	track.metadata = nil
	if got := track.GetSSRC(); got != 0 {
		t.Errorf("GetSSRC() with nil metadata = %v, want 0", got)
	}
}

func TestLocalTrack_GetMID(t *testing.T) {
	track := NewLocalTrack("pub-1", "room-1", TrackKindVideo, &webrtc.TrackRemote{}, &TrackMetadata{
		MID: "mid-1",
	})

	if got := track.GetMID(); got != "mid-1" {
		t.Errorf("GetMID() = %v, want %v", got, "mid-1")
	}

	// Test with nil metadata
	track.metadata = nil
	if got := track.GetMID(); got != "" {
		t.Errorf("GetMID() with nil metadata = %v, want empty string", got)
	}
}

func TestLocalTrack_IsSimulcast(t *testing.T) {
	tests := []struct {
		name      string
		simulcast bool
		want      bool
	}{
		{"simulcast enabled", true, true},
		{"simulcast disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			track := NewLocalTrack("pub-1", "room-1", TrackKindVideo, &webrtc.TrackRemote{}, &TrackMetadata{
				Simulcast: tt.simulcast,
			})

			if got := track.IsSimulcast(); got != tt.want {
				t.Errorf("IsSimulcast() = %v, want %v", got, tt.want)
			}
		})
	}

	// Test with nil metadata
	track := NewLocalTrack("pub-1", "room-1", TrackKindVideo, &webrtc.TrackRemote{}, nil)
	if got := track.IsSimulcast(); got != false {
		t.Errorf("IsSimulcast() with nil metadata = %v, want false", got)
	}
}

func TestLocalTrack_GetLayers(t *testing.T) {
	layers := []SimulcastLayer{SimulcastLayerHigh, SimulcastLayerMedium, SimulcastLayerLow}
	track := NewLocalTrack("pub-1", "room-1", TrackKindVideo, &webrtc.TrackRemote{}, &TrackMetadata{
		Layers: layers,
	})

	got := track.GetLayers()
	if len(got) != len(layers) {
		t.Errorf("GetLayers() length = %v, want %v", len(got), len(layers))
	}

	// Verify values
	for i, layer := range got {
		if layer != layers[i] {
			t.Errorf("GetLayers()[%d] = %v, want %v", i, layer, layers[i])
		}
	}

	// Verify it returns a copy (modifying returned value should not affect original)
	got[0] = SimulcastLayerLow
	if track.metadata.Layers[0] == SimulcastLayerLow {
		t.Error("GetLayers() did not return a copy")
	}

	// Test with nil metadata
	track.metadata = nil
	if got := track.GetLayers(); got != nil {
		t.Errorf("GetLayers() with nil metadata = %v, want nil", got)
	}
}

func TestLocalTrack_Validate(t *testing.T) {
	validTrack := NewLocalTrack("pub-1", "room-1", TrackKindVideo, &webrtc.TrackRemote{}, &TrackMetadata{})

	tests := []struct {
		name    string
		setup   func(*LocalTrack)
		wantErr bool
	}{
		{
			name:    "valid track",
			setup:   func(t *LocalTrack) {},
			wantErr: false,
		},
		{
			name: "empty track ID",
			setup: func(t *LocalTrack) {
				t.ID = ""
			},
			wantErr: true,
		},
		{
			name: "invalid track kind",
			setup: func(t *LocalTrack) {
				t.Kind = TrackKind("invalid")
			},
			wantErr: true,
		},
		{
			name: "empty publisher ID",
			setup: func(t *LocalTrack) {
				t.PublisherID = ""
			},
			wantErr: true,
		},
		{
			name: "empty room ID",
			setup: func(t *LocalTrack) {
				t.RoomID = ""
			},
			wantErr: true,
		},
		{
			name: "nil webrtc track",
			setup: func(t *LocalTrack) {
				t.Track = nil
			},
			wantErr: true,
		},
		{
			name: "simulcast enabled but no layers",
			setup: func(t *LocalTrack) {
				t.metadata = &TrackMetadata{
					Simulcast: true,
					Layers:    []SimulcastLayer{},
				}
			},
			wantErr: true,
		},
		{
			name: "simulcast with invalid layer",
			setup: func(t *LocalTrack) {
				t.metadata = &TrackMetadata{
					Simulcast: true,
					Layers:    []SimulcastLayer{SimulcastLayerHigh, SimulcastLayer("invalid")},
				}
			},
			wantErr: true,
		},
		{
			name: "simulcast with valid layers",
			setup: func(t *LocalTrack) {
				t.metadata = &TrackMetadata{
					Simulcast: true,
					Layers:    []SimulcastLayer{SimulcastLayerHigh, SimulcastLayerMedium, SimulcastLayerLow},
				}
			},
			wantErr: false,
		},
		{
			name: "nil metadata",
			setup: func(t *LocalTrack) {
				t.metadata = nil
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			track := NewLocalTrack(validTrack.PublisherID, validTrack.RoomID, validTrack.Kind,
				&webrtc.TrackRemote{}, &TrackMetadata{})
			tt.setup(track)

			err := track.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLocalTrack_ToTrackInfo(t *testing.T) {
	metadata := &TrackMetadata{
		Label:     "camera",
		Simulcast: true,
		Layers:    []SimulcastLayer{SimulcastLayerHigh, SimulcastLayerMedium},
		MID:       "mid-1",
		Custom:    map[string]interface{}{"key": "value"},
	}
	track := NewLocalTrack("pub-1", "room-1", TrackKindVideo, &webrtc.TrackRemote{}, metadata)

	info := track.ToTrackInfo()

	if info.ID != track.ID.String() {
		t.Errorf("ToTrackInfo() ID = %v, want %v", info.ID, track.ID.String())
	}
	if info.Kind != track.Kind.String() {
		t.Errorf("ToTrackInfo() Kind = %v, want %v", info.Kind, track.Kind.String())
	}
	if info.PublisherID != track.PublisherID {
		t.Errorf("ToTrackInfo() PublisherID = %v, want %v", info.PublisherID, track.PublisherID)
	}
	if info.Label != metadata.Label {
		t.Errorf("ToTrackInfo() Label = %v, want %v", info.Label, metadata.Label)
	}
	if info.Simulcast != metadata.Simulcast {
		t.Errorf("ToTrackInfo() Simulcast = %v, want %v", info.Simulcast, metadata.Simulcast)
	}
	if info.MID != metadata.MID {
		t.Errorf("ToTrackInfo() MID = %v, want %v", info.MID, metadata.MID)
	}
	if len(info.Layers) != len(metadata.Layers) {
		t.Errorf("ToTrackInfo() Layers length = %v, want %v", len(info.Layers), len(metadata.Layers))
	}
	if info.Custom == nil {
		t.Error("ToTrackInfo() Custom is nil")
	}
}

func TestLocalTrack_ConcurrentAccess(t *testing.T) {
	track := NewLocalTrack("pub-1", "room-1", TrackKindVideo, &webrtc.TrackRemote{}, &TrackMetadata{
		Label: "initial",
	})

	var wg sync.WaitGroup
	concurrency := 10

	// Concurrent reads
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = track.GetMetadata()
			_ = track.GetSSRC()
			_ = track.GetMID()
			_ = track.IsSimulcast()
			_ = track.GetLayers()
		}()
	}

	// Concurrent writes
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			track.UpdateMetadata(&TrackMetadata{
				Label: "updated",
				SSRC:  uint32(n),
			})
		}(i)
	}

	wg.Wait()

	// Verify the track is still valid
	if err := track.Validate(); err != nil {
		t.Errorf("Track is invalid after concurrent access: %v", err)
	}
}

func TestNewLocalTrack_DeepCopyMetadata(t *testing.T) {
	originalMetadata := &TrackMetadata{
		Label:     "camera",
		Simulcast: true,
		Layers:    []SimulcastLayer{SimulcastLayerHigh, SimulcastLayerMedium},
		MID:       "mid-1",
		SSRC:      12345,
		Custom:    map[string]interface{}{"key": "value"},
	}

	track := NewLocalTrack("pub-1", "room-1", TrackKindVideo, &webrtc.TrackRemote{}, originalMetadata)

	// Modify the original metadata
	originalMetadata.Label = "modified"
	originalMetadata.Layers[0] = SimulcastLayerLow
	originalMetadata.Custom["key"] = "modified"

	// Verify track metadata is not affected
	if track.metadata.Label == "modified" {
		t.Error("NewLocalTrack did not create a deep copy of metadata - Label was modified")
	}
	if track.metadata.Layers[0] == SimulcastLayerLow {
		t.Error("NewLocalTrack did not create a deep copy of metadata - Layers was modified")
	}
	if track.metadata.Custom["key"] == "modified" {
		t.Error("NewLocalTrack did not create a deep copy of metadata - Custom was modified")
	}
}

func TestLocalTrack_ToTrackInfo_DeepCopyCustom(t *testing.T) {
	metadata := &TrackMetadata{
		Label:  "camera",
		Custom: map[string]interface{}{"key": "original"},
	}
	track := NewLocalTrack("pub-1", "room-1", TrackKindVideo, &webrtc.TrackRemote{}, metadata)

	info := track.ToTrackInfo()

	// Modify the returned Custom map
	info.Custom["key"] = "modified"
	info.Custom["newKey"] = "newValue"

	// Verify track metadata is not affected
	if track.metadata.Custom["key"] != "original" {
		t.Error("ToTrackInfo did not create a deep copy of Custom map - existing key was modified")
	}
	if _, exists := track.metadata.Custom["newKey"]; exists {
		t.Error("ToTrackInfo did not create a deep copy of Custom map - new key was added")
	}
}

func TestNewLocalTrack_NilMetadata(t *testing.T) {
	track := NewLocalTrack("pub-1", "room-1", TrackKindVideo, &webrtc.TrackRemote{}, nil)

	if track.metadata != nil {
		t.Error("NewLocalTrack with nil metadata should have nil metadata field")
	}

	// Should still be valid (nil metadata is allowed)
	if err := track.Validate(); err != nil {
		t.Errorf("Track with nil metadata should be valid, got error: %v", err)
	}
}

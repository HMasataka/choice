package svc

import (
	"encoding/binary"
	"fmt"
)

// DependencyDescriptor represents the AV1 Dependency Descriptor RTP extension.
// This extension provides information about temporal and spatial scalability
// in VP9 and AV1 video streams.
// See: https://aomediacodec.github.io/av1-rtp-spec/#dependency-descriptor-rtp-header-extension
type DependencyDescriptor struct {
	// StartOfFrame indicates if this is the first packet of a frame.
	StartOfFrame bool
	// EndOfFrame indicates if this is the last packet of a frame.
	EndOfFrame bool
	// FrameNumber is the frame number (mod 2^16).
	FrameNumber uint16
	// TemplateDependencyStructurePresent indicates if the structure is present.
	TemplateDependencyStructurePresent bool
	// ActiveDecodeTargetsPresent indicates if active decode targets are present.
	ActiveDecodeTargetsPresent bool
	// CustomDTIsPresent indicates if custom DTIs are present.
	CustomDTIsPresent bool
	// CustomFDiffsPresent indicates if custom frame diffs are present.
	CustomFDiffsPresent bool
	// CustomChainsPresent indicates if custom chains are present.
	CustomChainsPresent bool
	// FrameDependencyTemplateID is the template ID for this frame.
	FrameDependencyTemplateID uint8
	// SpatialID is the spatial layer index (0-2).
	SpatialID int
	// TemporalID is the temporal layer index (0-2).
	TemporalID int
	// DecodeTargetIndications contains DTI for each decode target.
	DecodeTargetIndications []uint8
}

// GetSVCLayer returns the SVCLayer from the descriptor.
func (d *DependencyDescriptor) GetSVCLayer() SVCLayer {
	return SVCLayer{
		SpatialLayer:  d.SpatialID,
		TemporalLayer: d.TemporalID,
	}
}

// IsKeyFrame returns true if this is a keyframe (S0T0 with StartOfFrame).
func (d *DependencyDescriptor) IsKeyFrame() bool {
	return d.StartOfFrame && d.SpatialID == 0 && d.TemporalID == 0
}

// FrameDependencyTemplate represents a frame dependency template.
type FrameDependencyTemplate struct {
	SpatialID  int
	TemporalID int
	DTIs       []uint8 // Decode Target Indications
	FDiffs     []int   // Frame diffs to reference frames
	ChainDiffs []int   // Chain diffs
}

// TemplateDependencyStructure represents the template dependency structure.
type TemplateDependencyStructure struct {
	TemplateIDOffset     uint8
	DTCount              int                       // Decode Target Count
	MaxSpatialID         int                       // Max spatial ID (0-2)
	MaxTemporalID        int                       // Max temporal ID (0-2)
	Templates            []FrameDependencyTemplate // Frame dependency templates
	DecodeTargetLayers   []SVCLayer                // Mapping from DT to SVC layer
	MaxRenderResolutions []struct {
		Width  int
		Height int
	}
}

// DependencyDescriptorParser parses AV1 Dependency Descriptor RTP extensions.
//
// Limitations:
//   - Template dependency structure parsing is simplified: assumes templates are
//     arranged in a dense spatial×temporal grid based on MaxSpatialID/MaxTemporalID.
//   - Per-template fields (DTIs, FDiffs, chain diffs) are not parsed.
//   - This implementation is suitable for standard VP9/AV1 SVC streams with
//     uniform L3T3 scalability modes but may not handle non-standard configurations.
type DependencyDescriptorParser struct {
	// lastStructure holds the last parsed template dependency structure.
	// This is needed because the structure is not sent with every packet.
	lastStructure *TemplateDependencyStructure
}

// NewDependencyDescriptorParser creates a new parser.
func NewDependencyDescriptorParser() *DependencyDescriptorParser {
	return &DependencyDescriptorParser{}
}

// Parse parses a dependency descriptor from raw bytes.
// The input is expected to be the raw RTP extension data.
func (p *DependencyDescriptorParser) Parse(data []byte) (*DependencyDescriptor, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("dependency descriptor too short: need at least 1 byte, got %d", len(data))
	}

	dd := &DependencyDescriptor{}

	// Mandatory descriptor fields (first byte)
	firstByte := data[0]
	dd.StartOfFrame = (firstByte & 0x80) != 0
	dd.EndOfFrame = (firstByte & 0x40) != 0
	dd.TemplateDependencyStructurePresent = (firstByte & 0x20) != 0
	dd.ActiveDecodeTargetsPresent = (firstByte & 0x10) != 0
	dd.CustomDTIsPresent = (firstByte & 0x08) != 0
	dd.CustomFDiffsPresent = (firstByte & 0x04) != 0
	dd.CustomChainsPresent = (firstByte & 0x02) != 0

	offset := 1

	// Frame number (if template dependency structure is present or this is the first packet)
	if dd.TemplateDependencyStructurePresent || dd.StartOfFrame {
		if len(data) < offset+2 {
			return nil, fmt.Errorf("dependency descriptor too short for frame number")
		}
		dd.FrameNumber = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
	}

	// Parse template dependency structure if present
	if dd.TemplateDependencyStructurePresent {
		structure, consumed, err := p.parseTemplateDependencyStructure(data[offset:])
		if err != nil {
			return nil, fmt.Errorf("failed to parse template dependency structure: %w", err)
		}
		p.lastStructure = structure
		offset += consumed
	}

	// Frame dependency template ID
	if len(data) > offset {
		// Template ID is encoded in lower 6 bits
		dd.FrameDependencyTemplateID = data[offset] & 0x3F
		offset++
	}

	// Derive spatial and temporal IDs from template
	if p.lastStructure != nil && len(p.lastStructure.Templates) > 0 {
		templateIdx := int(dd.FrameDependencyTemplateID) - int(p.lastStructure.TemplateIDOffset)
		if templateIdx >= 0 && templateIdx < len(p.lastStructure.Templates) {
			template := p.lastStructure.Templates[templateIdx]
			dd.SpatialID = template.SpatialID
			dd.TemporalID = template.TemporalID
		}
	}

	return dd, nil
}

// parseTemplateDependencyStructure parses the template dependency structure.
func (p *DependencyDescriptorParser) parseTemplateDependencyStructure(data []byte) (*TemplateDependencyStructure, int, error) {
	if len(data) < 1 {
		return nil, 0, fmt.Errorf("template dependency structure too short")
	}

	structure := &TemplateDependencyStructure{}
	offset := 0

	// template_id_offset (6 bits) + dt_cnt_minus_one (5 bits) = 11 bits
	if len(data) < offset+2 {
		return nil, 0, fmt.Errorf("template dependency structure too short for header")
	}

	// First byte: template_id_offset (6 bits) + 2 bits of dt_cnt
	structure.TemplateIDOffset = data[offset] >> 2
	dtCntMinusOne := int((data[offset]&0x03)<<3) | int(data[offset+1]>>5)
	structure.DTCount = dtCntMinusOne + 1

	// max_spatial_id (2 bits) + max_temporal_id (3 bits)
	structure.MaxSpatialID = int((data[offset+1] >> 3) & 0x03)
	structure.MaxTemporalID = int(data[offset+1] & 0x07)
	offset += 2

	// Parse template count
	templateCnt := (structure.MaxSpatialID + 1) * (structure.MaxTemporalID + 1)
	structure.Templates = make([]FrameDependencyTemplate, templateCnt)

	// Initialize templates with default spatial/temporal IDs
	idx := 0
	for s := 0; s <= structure.MaxSpatialID; s++ {
		for t := 0; t <= structure.MaxTemporalID; t++ {
			if idx < len(structure.Templates) {
				structure.Templates[idx].SpatialID = s
				structure.Templates[idx].TemporalID = t
				idx++
			}
		}
	}

	// Build decode target layers mapping
	structure.DecodeTargetLayers = make([]SVCLayer, structure.DTCount)
	for i := 0; i < structure.DTCount && i < len(structure.Templates); i++ {
		structure.DecodeTargetLayers[i] = SVCLayer{
			SpatialLayer:  structure.Templates[i%len(structure.Templates)].SpatialID,
			TemporalLayer: structure.Templates[i%len(structure.Templates)].TemporalID,
		}
	}

	return structure, offset, nil
}

// GetLastStructure returns the last parsed template dependency structure.
func (p *DependencyDescriptorParser) GetLastStructure() *TemplateDependencyStructure {
	return p.lastStructure
}

// Reset clears the parser state.
func (p *DependencyDescriptorParser) Reset() {
	p.lastStructure = nil
}

// GetAvailableLayers returns the available SVC layers from the last structure.
func (p *DependencyDescriptorParser) GetAvailableLayers() []SVCLayer {
	if p.lastStructure == nil {
		return nil
	}

	var layers []SVCLayer
	seen := make(map[string]bool)

	for s := 0; s <= p.lastStructure.MaxSpatialID; s++ {
		for t := 0; t <= p.lastStructure.MaxTemporalID; t++ {
			layer := SVCLayer{SpatialLayer: s, TemporalLayer: t}
			key := layer.String()
			if !seen[key] {
				seen[key] = true
				layers = append(layers, layer)
			}
		}
	}

	return layers
}

// GetMaxSpatialLayer returns the maximum spatial layer from the last structure.
func (p *DependencyDescriptorParser) GetMaxSpatialLayer() int {
	if p.lastStructure == nil {
		return 0
	}
	return p.lastStructure.MaxSpatialID
}

// GetMaxTemporalLayer returns the maximum temporal layer from the last structure.
func (p *DependencyDescriptorParser) GetMaxTemporalLayer() int {
	if p.lastStructure == nil {
		return 0
	}
	return p.lastStructure.MaxTemporalID
}

// DependencyDescriptorExtensionID is the RTP extension ID for dependency descriptor.
// This is typically negotiated via SDP, but the common value is shown here.
const DependencyDescriptorExtensionURI = "https://aomediacodec.github.io/av1-rtp-spec/#dependency-descriptor-rtp-header-extension"

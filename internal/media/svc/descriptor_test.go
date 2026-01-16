package svc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDependencyDescriptorGetSVCLayer(t *testing.T) {
	dd := &DependencyDescriptor{
		SpatialID:  2,
		TemporalID: 1,
	}

	layer := dd.GetSVCLayer()
	assert.Equal(t, 2, layer.SpatialLayer)
	assert.Equal(t, 1, layer.TemporalLayer)
}

func TestDependencyDescriptorIsKeyFrame(t *testing.T) {
	t.Run("keyframe at S0T0 with StartOfFrame", func(t *testing.T) {
		dd := &DependencyDescriptor{
			StartOfFrame: true,
			SpatialID:    0,
			TemporalID:   0,
		}
		assert.True(t, dd.IsKeyFrame())
	})

	t.Run("not keyframe if not StartOfFrame", func(t *testing.T) {
		dd := &DependencyDescriptor{
			StartOfFrame: false,
			SpatialID:    0,
			TemporalID:   0,
		}
		assert.False(t, dd.IsKeyFrame())
	})

	t.Run("not keyframe if not S0T0", func(t *testing.T) {
		dd := &DependencyDescriptor{
			StartOfFrame: true,
			SpatialID:    1,
			TemporalID:   0,
		}
		assert.False(t, dd.IsKeyFrame())
	})
}

func TestNewDependencyDescriptorParser(t *testing.T) {
	parser := NewDependencyDescriptorParser()
	assert.NotNil(t, parser)
	assert.Nil(t, parser.GetLastStructure())
}

func TestDependencyDescriptorParserParse(t *testing.T) {
	t.Run("parses basic descriptor", func(t *testing.T) {
		parser := NewDependencyDescriptorParser()

		// Construct a minimal descriptor:
		// Byte 0: StartOfFrame=1, EndOfFrame=1, rest=0 -> 0xC0
		data := []byte{0xC0, 0x00, 0x01} // Start+End, frame number 1

		dd, err := parser.Parse(data)
		require.NoError(t, err)
		assert.True(t, dd.StartOfFrame)
		assert.True(t, dd.EndOfFrame)
		assert.False(t, dd.TemplateDependencyStructurePresent)
	})

	t.Run("parses descriptor with template structure present flag", func(t *testing.T) {
		parser := NewDependencyDescriptorParser()

		// Byte 0: StartOfFrame=1, EndOfFrame=1, TemplateDependencyStructurePresent=1 -> 0xE0
		// Bytes 1-2: Frame number
		// Bytes 3+: Template structure
		data := []byte{
			0xE0,       // Flags: start, end, template present
			0x00, 0x01, // Frame number
			0x00, // Template ID offset (6 bits) + dt_cnt high bits
			0x00, // dt_cnt low bits + max_spatial + max_temporal
		}

		dd, err := parser.Parse(data)
		require.NoError(t, err)
		assert.True(t, dd.StartOfFrame)
		assert.True(t, dd.EndOfFrame)
		assert.True(t, dd.TemplateDependencyStructurePresent)
	})

	t.Run("parses template ID when structure present", func(t *testing.T) {
		parser := NewDependencyDescriptorParser()

		// TemplateIDOffset=0, MaxSpatialID=1, MaxTemporalID=0, TemplateID=1
		data := []byte{
			0xE0,       // Flags: start, end, template present
			0x00, 0x01, // Frame number
			0x00, // Template ID offset (6 bits) + dt_cnt high bits
			0x08, // dt_cnt low bits + max_spatial=1 + max_temporal=0
			0x01, // Frame dependency template ID
		}

		dd, err := parser.Parse(data)
		require.NoError(t, err)
		assert.Equal(t, uint8(1), dd.FrameDependencyTemplateID)
		assert.Equal(t, 1, dd.SpatialID)
		assert.Equal(t, 0, dd.TemporalID)
	})

	t.Run("does not underflow template index with non-zero offset", func(t *testing.T) {
		parser := NewDependencyDescriptorParser()

		// TemplateIDOffset=1, MaxSpatialID=1, MaxTemporalID=0, TemplateID=0
		data := []byte{
			0xE0,       // Flags: start, end, template present
			0x00, 0x01, // Frame number
			0x04, // Template ID offset=1 (6 bits) + dt_cnt high bits
			0x08, // dt_cnt low bits + max_spatial=1 + max_temporal=0
			0x00, // Frame dependency template ID
		}

		dd, err := parser.Parse(data)
		require.NoError(t, err)
		assert.Equal(t, uint8(0), dd.FrameDependencyTemplateID)
		assert.Equal(t, 0, dd.SpatialID)
		assert.Equal(t, 0, dd.TemporalID)
	})

	t.Run("returns error for empty data", func(t *testing.T) {
		parser := NewDependencyDescriptorParser()
		_, err := parser.Parse([]byte{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too short")
	})

	t.Run("returns error for truncated frame number", func(t *testing.T) {
		parser := NewDependencyDescriptorParser()
		// StartOfFrame set but no frame number bytes
		data := []byte{0x80}
		_, err := parser.Parse(data)
		assert.Error(t, err)
	})
}

func TestDependencyDescriptorParserReset(t *testing.T) {
	parser := NewDependencyDescriptorParser()

	// Parse something to set lastStructure
	data := []byte{
		0xE0,       // Flags with template present
		0x00, 0x01, // Frame number
		0x00, 0x00, // Template structure header
	}
	_, _ = parser.Parse(data)

	parser.Reset()
	assert.Nil(t, parser.GetLastStructure())
}

func TestDependencyDescriptorParserGetAvailableLayers(t *testing.T) {
	t.Run("returns nil when no structure parsed", func(t *testing.T) {
		parser := NewDependencyDescriptorParser()
		layers := parser.GetAvailableLayers()
		assert.Nil(t, layers)
	})

	t.Run("returns layers from structure", func(t *testing.T) {
		parser := NewDependencyDescriptorParser()

		// Manually set a structure
		parser.lastStructure = &TemplateDependencyStructure{
			MaxSpatialID:  2,
			MaxTemporalID: 2,
		}

		layers := parser.GetAvailableLayers()
		assert.Len(t, layers, 9) // 3x3 = 9 layers

		// Verify some specific layers are present
		found := make(map[string]bool)
		for _, l := range layers {
			found[l.String()] = true
		}

		assert.True(t, found["S0T0"])
		assert.True(t, found["S2T2"])
		assert.True(t, found["S1T1"])
	})
}

func TestDependencyDescriptorParserGetMaxLayers(t *testing.T) {
	t.Run("returns 0 when no structure", func(t *testing.T) {
		parser := NewDependencyDescriptorParser()
		assert.Equal(t, 0, parser.GetMaxSpatialLayer())
		assert.Equal(t, 0, parser.GetMaxTemporalLayer())
	})

	t.Run("returns values from structure", func(t *testing.T) {
		parser := NewDependencyDescriptorParser()
		parser.lastStructure = &TemplateDependencyStructure{
			MaxSpatialID:  2,
			MaxTemporalID: 3,
		}

		assert.Equal(t, 2, parser.GetMaxSpatialLayer())
		assert.Equal(t, 3, parser.GetMaxTemporalLayer())
	})
}

func TestTemplateDependencyStructure(t *testing.T) {
	structure := &TemplateDependencyStructure{
		TemplateIDOffset: 0,
		DTCount:          3,
		MaxSpatialID:     2,
		MaxTemporalID:    2,
		Templates: []FrameDependencyTemplate{
			{SpatialID: 0, TemporalID: 0},
			{SpatialID: 1, TemporalID: 1},
			{SpatialID: 2, TemporalID: 2},
		},
		DecodeTargetLayers: []SVCLayer{
			{SpatialLayer: 0, TemporalLayer: 0},
			{SpatialLayer: 1, TemporalLayer: 1},
			{SpatialLayer: 2, TemporalLayer: 2},
		},
	}

	assert.Equal(t, 3, structure.DTCount)
	assert.Equal(t, 2, structure.MaxSpatialID)
	assert.Equal(t, 2, structure.MaxTemporalID)
	assert.Len(t, structure.Templates, 3)
}

func TestDependencyDescriptorExtensionURI(t *testing.T) {
	assert.NotEmpty(t, DependencyDescriptorExtensionURI)
	assert.Contains(t, DependencyDescriptorExtensionURI, "dependency-descriptor")
}

package sfu

import "time"

const (
	quarterResolution = "q"
	halfResolution    = "h"
	fullResolution    = "f"
)

type SimulcastConfig struct {
	BestQualityFirst    bool `toml:"bestqualityfirst"`
	EnableTemporalLayer bool `toml:"enabletemporallayer"`
}

type SimulcastTrackHelpers struct {
	switchDelay       time.Time
	temporalSupported bool
	temporalEnabled   bool
	lTSCalc           int64

	// VP8Helper temporal helpers
	pRefPicID  uint16
	refPicID   uint16
	lPicID     uint16
	pRefTlZIdx uint8
	refTlZIdx  uint8
	lTlZIdx    uint8
	refSN      uint16
}

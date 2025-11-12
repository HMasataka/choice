package sfu

type SimulcastConfig struct {
	BestQualityFirst    bool `mapstructure:"bestqualityfirst"`
	EnableTemporalLayer bool `mapstructure:"enabletemporallayer"`
}

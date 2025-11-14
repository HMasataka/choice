package sfu

import (
	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

// DownTrackはSubscriberにメディアを送信するための抽象化された構造体です。
// DownTrackはReceiverから受信したメディアをSubscriberに配信します。
// SubscriberとDownTrackは1対多の関係です。
type DownTrack interface {
	Bind(t webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error)
	Unbind(_ webrtc.TrackLocalContext) error
	ID() string
	Codec() webrtc.RTPCodecCapability
	StreamID() string
	RID() string
	Kind() webrtc.RTPCodecType
	Stop() error
	SetTransceiver(transceiver *webrtc.RTPTransceiver)
	WriteRTP(p *buffer.ExtPacket, layer int) error
	Enabled() bool
	Mute(val bool)
	Close()
	SetInitialLayers(spatialLayer, temporalLayer int32)
	CurrentSpatialLayer() int
	SwitchSpatialLayer(targetLayer int32, setAsMax bool) error
	SwitchSpatialLayerDone(layer int32)
	UptrackLayersChange(availableLayers []uint16) (int64, error)
	SwitchTemporalLayer(targetLayer int32, setAsMax bool)
	OnCloseHandler(fn func())
	OnBind(fn func())
	CreateSourceDescriptionChunks() []rtcp.SourceDescriptionChunk
	CreateSenderReport() *rtcp.SenderReport
	UpdateStats(packetLen uint32)
}

type downTrack struct {
}

func NewDownTrack() *downTrack {
	return &downTrack{}
}

var _ DownTrack = (*downTrack)(nil)

func (d *downTrack) Bind(t webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	return webrtc.RTPCodecParameters{}, nil
}

func (d *downTrack) Unbind(_ webrtc.TrackLocalContext) error {
	return nil
}

func (d *downTrack) ID() string {
	return ""
}

func (d *downTrack) Codec() webrtc.RTPCodecCapability {
	return webrtc.RTPCodecCapability{}
}

func (d *downTrack) StreamID() string {
	return ""
}

func (d *downTrack) RID() string {
	return ""
}

func (d *downTrack) Kind() webrtc.RTPCodecType {
	return webrtc.RTPCodecType(0)
}

func (d *downTrack) Stop() error {
	return nil
}

func (d *downTrack) SetTransceiver(transceiver *webrtc.RTPTransceiver) {
}

func (d *downTrack) WriteRTP(p *buffer.ExtPacket, layer int) error {
	return nil
}

func (d *downTrack) Enabled() bool {
	return false
}

func (d *downTrack) Mute(val bool) {
}

func (d *downTrack) Close() {
}

func (d *downTrack) SetInitialLayers(spatialLayer, temporalLayer int32) {
}

func (d *downTrack) CurrentSpatialLayer() int {
	return 0
}

func (d *downTrack) SwitchSpatialLayer(targetLayer int32, setAsMax bool) error {
	return nil
}

func (d *downTrack) SwitchSpatialLayerDone(layer int32) {
}

func (d *downTrack) UptrackLayersChange(availableLayers []uint16) (int64, error) {
	return 0, nil
}

func (d *downTrack) SwitchTemporalLayer(targetLayer int32, setAsMax bool) {
}

func (d *downTrack) OnCloseHandler(fn func()) {
}

func (d *downTrack) OnBind(fn func()) {
}

func (d *downTrack) CreateSourceDescriptionChunks() []rtcp.SourceDescriptionChunk {
	return nil
}

func (d *downTrack) CreateSenderReport() *rtcp.SenderReport {
	return &rtcp.SenderReport{}
}

func (d *downTrack) UpdateStats(packetLen uint32) {
}

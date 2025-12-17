package sfu

import (
	"slices"
	"sort"
	"sync"
)

type AudioStream struct {
	ID               string
	AccumulatedLevel int
	ActiveCount      int
}

// AudioObserver 音声レベル（dBov）を追跡し、アクティブスピーカー検出（誰が話しているかを検出する機能）を実現します。
// 閾値とフィルター設定により、ノイズや短い発話を除外できます。
type AudioObserver struct {
	mu                sync.RWMutex
	streams           []*AudioStream
	expectedCount     int
	threshold         uint8
	previousStreamIDs []string
}

func NewAudioObserver(threshold uint8, interval, filter int) *AudioObserver {
	threshold = min(threshold, 127)

	filter = max(filter, 0)
	filter = min(filter, 100)

	return &AudioObserver{
		threshold:     threshold,
		expectedCount: interval * filter / 2000,
	}
}

func (a *AudioObserver) addStream(streamID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.streams = append(a.streams, &AudioStream{ID: streamID})
}

func (a *AudioObserver) removeStream(streamID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.streams = slices.DeleteFunc(a.streams, func(s *AudioStream) bool {
		return s.ID == streamID
	})
}

func (a *AudioObserver) observe(streamID string, dBov uint8) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, as := range a.streams {
		if as.ID != streamID {
			continue
		}

		if dBov <= a.threshold {
			as.AccumulatedLevel += int(dBov)
			as.ActiveCount++
		}

		return
	}
}

// sortStreamsByActivity は音声ストリームを活動レベル順にソートします (ActiveCount降順、AccumulatedLevel昇順)
func (a *AudioObserver) sortStreamsByActivity(streams []*AudioStream) []*AudioStream {
	sort.Slice(streams, func(i, j int) bool {
		si, sj := streams[i], streams[j]

		if si.ActiveCount != sj.ActiveCount {
			return si.ActiveCount > sj.ActiveCount
		}

		return si.AccumulatedLevel < sj.AccumulatedLevel
	})

	return streams
}

// changedStreamIDs は前回の結果と比較し、変化があればstreamIDsを返却、変化がなければnilを返却します
func (a *AudioObserver) changedStreamIDs(previous, streamIDs []string) []string {
	if len(previous) != len(streamIDs) {
		return streamIDs
	}

	for i, stream := range previous {
		if stream != streamIDs[i] {
			return streamIDs
		}
	}

	return nil
}

func (a *AudioObserver) Calc() []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.streams = a.sortStreamsByActivity(a.streams)

	streamIDs := make([]string, 0, len(a.streams))

	for _, stream := range a.streams {
		if stream.ActiveCount >= a.expectedCount {
			streamIDs = append(streamIDs, stream.ID)
		}
		stream.ActiveCount = 0
		stream.AccumulatedLevel = 0
	}

	changedStreamIDs := a.changedStreamIDs(a.previousStreamIDs, streamIDs)
	if changedStreamIDs == nil {
		return nil
	}

	a.previousStreamIDs = streamIDs

	return streamIDs
}

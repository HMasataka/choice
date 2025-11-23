package sfu

import (
	"slices"
	"sort"
	"sync"
)

type AudioStream struct {
	ID    string
	Sum   int
	Total int
}

// AudioObserver 音声レベル（dBov）を追跡し、アクティブスピーカー検出（誰が話しているかを検出する機能）を実現します。
// 閾値とフィルター設定により、ノイズや短い発話を除外できます。
type AudioObserver struct {
	sync.RWMutex
	streams   []*AudioStream
	expected  int
	threshold uint8
	previous  []string
}

func NewAudioObserver(threshold uint8, interval, filter int) *AudioObserver {
	if threshold > 127 {
		threshold = 127
	}
	if filter < 0 {
		filter = 0
	}
	if filter > 100 {
		filter = 100
	}

	return &AudioObserver{
		threshold: threshold,
		expected:  interval * filter / 2000,
	}
}

func (a *AudioObserver) addStream(streamID string) {
	a.Lock()
	defer a.Unlock()

	a.streams = append(a.streams, &AudioStream{ID: streamID})
}

func (a *AudioObserver) removeStream(streamID string) {
	a.Lock()
	defer a.Unlock()

	a.streams = slices.DeleteFunc(a.streams, func(s *AudioStream) bool {
		return s.ID == streamID
	})
}

func (a *AudioObserver) observe(streamID string, dBov uint8) {
	a.RLock()
	defer a.RUnlock()

	for _, as := range a.streams {
		if as.ID != streamID {
			continue
		}

		if dBov <= a.threshold {
			as.Sum += int(dBov)
			as.Total++
		}

		return
	}
}

// sortStreamsByActivity は音声ストリームを活動レベル順にソートします (total降順、sum昇順)
func (a *AudioObserver) sortStreamsByActivity(streams []*AudioStream) []*AudioStream {
	sort.Slice(streams, func(i, j int) bool {
		si, sj := streams[i], streams[j]

		if si.Total != sj.Total {
			return si.Total > sj.Total
		}

		return si.Sum < sj.Sum
	})

	return streams
}

func (a *AudioObserver) Calc() []string {
	a.Lock()
	defer a.Unlock()

	a.streams = a.sortStreamsByActivity(a.streams)

	streamIDs := make([]string, 0, len(a.streams))

	for _, stream := range a.streams {
		if stream.Total >= a.expected {
			streamIDs = append(streamIDs, stream.ID)
		}
		stream.Total = 0
		stream.Sum = 0
	}

	if len(a.previous) == len(streamIDs) {
		for i, s := range a.previous {
			if s != streamIDs[i] {
				a.previous = streamIDs
				return streamIDs
			}
		}
		return nil
	}

	a.previous = streamIDs
	return streamIDs
}

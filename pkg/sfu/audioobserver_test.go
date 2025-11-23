package sfu_test

import (
	"testing"

	"github.com/HMasataka/choice/pkg/sfu"
	"github.com/stretchr/testify/suite"
)

type AudioObserverTestSuit struct {
	target *sfu.AudioObserver
	suite.Suite
}

func TestAudioObserverTestSuit(t *testing.T) {
	suite.Run(t, &AudioObserverTestSuit{})
}

func (ts *AudioObserverTestSuit) SetupTest() {
	ts.target = sfu.NewAudioObserver(50, 100, 50)
}

func (ts *AudioObserverTestSuit) TearDownTest() {
}

func (ts *AudioObserverTestSuit) Test_Order_By_ActiveCount_Success() {
	stream := []*sfu.AudioStream{
		{ID: "id1", AccumulatedLevel: 100, ActiveCount: 10},
		{ID: "id2", AccumulatedLevel: 200, ActiveCount: 20},
		{ID: "id3", AccumulatedLevel: 300, ActiveCount: 30},
	}

	got := ts.target.ExportSortStreamsByActivity(stream)
	ts.Require().NotNil(got)
	ts.Equal("id3", got[0].ID)
	ts.Equal("id2", got[1].ID)
	ts.Equal("id1", got[2].ID)
}

func (ts *AudioObserverTestSuit) Test_Order_By_AccumulatedLevel_Success() {
	stream := []*sfu.AudioStream{
		{ID: "id1", AccumulatedLevel: 300, ActiveCount: 0},
		{ID: "id2", AccumulatedLevel: 200, ActiveCount: 0},
		{ID: "id3", AccumulatedLevel: 100, ActiveCount: 0},
	}

	got := ts.target.ExportSortStreamsByActivity(stream)
	ts.Require().NotNil(got)
	ts.Equal("id3", got[0].ID)
	ts.Equal("id2", got[1].ID)
	ts.Equal("id1", got[2].ID)
}

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

func (ts *AudioObserverTestSuit) Test_Order_By_Total_Success() {
	stream := []*sfu.AudioStream{
		{ID: "id1", Sum: 100, Total: 10},
		{ID: "id2", Sum: 200, Total: 20},
		{ID: "id3", Sum: 300, Total: 30},
	}

	got := ts.target.ExportSortStreamsByActivity(stream)
	ts.Require().NotNil(got)
	ts.Equal("id3", got[0].ID)
	ts.Equal("id2", got[1].ID)
	ts.Equal("id1", got[2].ID)
}

func (ts *AudioObserverTestSuit) Test_Order_By_Sum_Success() {
	stream := []*sfu.AudioStream{
		{ID: "id1", Sum: 300, Total: 0},
		{ID: "id2", Sum: 200, Total: 0},
		{ID: "id3", Sum: 100, Total: 0},
	}

	got := ts.target.ExportSortStreamsByActivity(stream)
	ts.Require().NotNil(got)
	ts.Equal("id3", got[0].ID)
	ts.Equal("id2", got[1].ID)
	ts.Equal("id1", got[2].ID)
}

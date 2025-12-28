package sfu

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAudioObserver(t *testing.T) {
	t.Run("基本的な初期化", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)

		require.NotNil(t, ao)
		assert.Equal(t, uint8(50), ao.threshold)
		// expectedCount = interval * filter / 2000 = 100 * 50 / 2000 = 2
		assert.Equal(t, 2, ao.expectedCount)
		assert.Empty(t, ao.streams)
		assert.Empty(t, ao.previousStreamIDs)
	})

	t.Run("閾値の上限クランプ", func(t *testing.T) {
		// threshold > 127 は 127 にクランプされる
		ao := NewAudioObserver(200, 100, 50)

		assert.Equal(t, uint8(127), ao.threshold)
	})

	t.Run("閾値の境界値", func(t *testing.T) {
		// ちょうど127はそのまま
		ao := NewAudioObserver(127, 100, 50)
		assert.Equal(t, uint8(127), ao.threshold)

		// 128は127にクランプ
		ao2 := NewAudioObserver(128, 100, 50)
		assert.Equal(t, uint8(127), ao2.threshold)
	})

	t.Run("フィルターの下限クランプ", func(t *testing.T) {
		// filter < 0 は 0 にクランプされる
		ao := NewAudioObserver(50, 100, -10)

		assert.Equal(t, 0, ao.expectedCount)
	})

	t.Run("フィルターの上限クランプ", func(t *testing.T) {
		// filter > 100 は 100 にクランプされる
		ao := NewAudioObserver(50, 100, 150)

		// expectedCount = 100 * 100 / 2000 = 5
		assert.Equal(t, 5, ao.expectedCount)
	})

	t.Run("expectedCount計算", func(t *testing.T) {
		testCases := []struct {
			interval      int
			filter        int
			expectedCount int
		}{
			{100, 50, 2},    // 100 * 50 / 2000 = 2
			{200, 50, 5},    // 200 * 50 / 2000 = 5
			{100, 100, 5},   // 100 * 100 / 2000 = 5
			{1000, 100, 50}, // 1000 * 100 / 2000 = 50
			{100, 0, 0},     // 100 * 0 / 2000 = 0
		}

		for _, tc := range testCases {
			ao := NewAudioObserver(50, tc.interval, tc.filter)
			assert.Equal(t, tc.expectedCount, ao.expectedCount)
		}
	})
}

func TestAudioObserver_AddStream(t *testing.T) {
	t.Run("ストリームの追加", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)

		ao.addStream("stream1")

		require.Len(t, ao.streams, 1)
		assert.Equal(t, "stream1", ao.streams[0].ID)
		assert.Equal(t, 0, ao.streams[0].AccumulatedLevel)
		assert.Equal(t, 0, ao.streams[0].ActiveCount)
	})

	t.Run("複数ストリームの追加", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)

		ao.addStream("stream1")
		ao.addStream("stream2")
		ao.addStream("stream3")

		require.Len(t, ao.streams, 3)
		assert.Equal(t, "stream1", ao.streams[0].ID)
		assert.Equal(t, "stream2", ao.streams[1].ID)
		assert.Equal(t, "stream3", ao.streams[2].ID)
	})

	t.Run("同じIDの重複追加", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)

		ao.addStream("stream1")
		ao.addStream("stream1")

		// 重複チェックはないので2つ追加される
		assert.Len(t, ao.streams, 2)
	})
}

func TestAudioObserver_RemoveStream(t *testing.T) {
	t.Run("ストリームの削除", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)
		ao.addStream("stream1")
		ao.addStream("stream2")
		ao.addStream("stream3")

		ao.removeStream("stream2")

		require.Len(t, ao.streams, 2)
		assert.Equal(t, "stream1", ao.streams[0].ID)
		assert.Equal(t, "stream3", ao.streams[1].ID)
	})

	t.Run("存在しないストリームの削除", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)
		ao.addStream("stream1")

		ao.removeStream("nonexistent")

		assert.Len(t, ao.streams, 1)
	})

	t.Run("空のリストからの削除", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)

		ao.removeStream("stream1")

		assert.Empty(t, ao.streams)
	})

	t.Run("先頭の削除", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)
		ao.addStream("stream1")
		ao.addStream("stream2")

		ao.removeStream("stream1")

		require.Len(t, ao.streams, 1)
		assert.Equal(t, "stream2", ao.streams[0].ID)
	})

	t.Run("末尾の削除", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)
		ao.addStream("stream1")
		ao.addStream("stream2")

		ao.removeStream("stream2")

		require.Len(t, ao.streams, 1)
		assert.Equal(t, "stream1", ao.streams[0].ID)
	})
}

func TestAudioObserver_Observe(t *testing.T) {
	t.Run("閾値以下のレベルを記録", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)
		ao.addStream("stream1")

		ao.observe("stream1", 30)

		assert.Equal(t, 30, ao.streams[0].AccumulatedLevel)
		assert.Equal(t, 1, ao.streams[0].ActiveCount)
	})

	t.Run("閾値ちょうどのレベルを記録", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)
		ao.addStream("stream1")

		ao.observe("stream1", 50)

		assert.Equal(t, 50, ao.streams[0].AccumulatedLevel)
		assert.Equal(t, 1, ao.streams[0].ActiveCount)
	})

	t.Run("閾値を超えるレベルは無視", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)
		ao.addStream("stream1")

		ao.observe("stream1", 60)

		assert.Equal(t, 0, ao.streams[0].AccumulatedLevel)
		assert.Equal(t, 0, ao.streams[0].ActiveCount)
	})

	t.Run("複数回のobserve", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)
		ao.addStream("stream1")

		ao.observe("stream1", 10)
		ao.observe("stream1", 20)
		ao.observe("stream1", 30)

		assert.Equal(t, 60, ao.streams[0].AccumulatedLevel) // 10 + 20 + 30
		assert.Equal(t, 3, ao.streams[0].ActiveCount)
	})

	t.Run("存在しないストリームへのobserve", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)
		ao.addStream("stream1")

		// パニックしないことを確認
		ao.observe("nonexistent", 30)

		assert.Equal(t, 0, ao.streams[0].AccumulatedLevel)
	})

	t.Run("複数ストリームへのobserve", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)
		ao.addStream("stream1")
		ao.addStream("stream2")

		ao.observe("stream1", 10)
		ao.observe("stream2", 20)
		ao.observe("stream1", 15)

		assert.Equal(t, 25, ao.streams[0].AccumulatedLevel)
		assert.Equal(t, 2, ao.streams[0].ActiveCount)
		assert.Equal(t, 20, ao.streams[1].AccumulatedLevel)
		assert.Equal(t, 1, ao.streams[1].ActiveCount)
	})
}

func TestAudioObserver_SortStreamsByActivity(t *testing.T) {
	ao := NewAudioObserver(50, 100, 50)

	t.Run("ActiveCount降順でソート", func(t *testing.T) {
		streams := []*AudioStream{
			{ID: "id1", AccumulatedLevel: 100, ActiveCount: 10},
			{ID: "id2", AccumulatedLevel: 200, ActiveCount: 20},
			{ID: "id3", AccumulatedLevel: 300, ActiveCount: 30},
		}

		result := ao.sortStreamsByActivity(streams)

		require.Len(t, result, 3)
		assert.Equal(t, "id3", result[0].ID)
		assert.Equal(t, "id2", result[1].ID)
		assert.Equal(t, "id1", result[2].ID)
	})

	t.Run("ActiveCount同じ場合はAccumulatedLevel昇順", func(t *testing.T) {
		streams := []*AudioStream{
			{ID: "id1", AccumulatedLevel: 300, ActiveCount: 10},
			{ID: "id2", AccumulatedLevel: 100, ActiveCount: 10},
			{ID: "id3", AccumulatedLevel: 200, ActiveCount: 10},
		}

		result := ao.sortStreamsByActivity(streams)

		require.Len(t, result, 3)
		assert.Equal(t, "id2", result[0].ID) // 100
		assert.Equal(t, "id3", result[1].ID) // 200
		assert.Equal(t, "id1", result[2].ID) // 300
	})

	t.Run("混合ケース", func(t *testing.T) {
		streams := []*AudioStream{
			{ID: "id1", AccumulatedLevel: 100, ActiveCount: 5},
			{ID: "id2", AccumulatedLevel: 50, ActiveCount: 10},
			{ID: "id3", AccumulatedLevel: 200, ActiveCount: 10},
			{ID: "id4", AccumulatedLevel: 150, ActiveCount: 5},
		}

		result := ao.sortStreamsByActivity(streams)

		require.Len(t, result, 4)
		// ActiveCount=10のグループがまず来る（AccumulatedLevel昇順）
		assert.Equal(t, "id2", result[0].ID) // count=10, level=50
		assert.Equal(t, "id3", result[1].ID) // count=10, level=200
		// ActiveCount=5のグループが次（AccumulatedLevel昇順）
		assert.Equal(t, "id1", result[2].ID) // count=5, level=100
		assert.Equal(t, "id4", result[3].ID) // count=5, level=150
	})

	t.Run("空のスライス", func(t *testing.T) {
		streams := []*AudioStream{}

		result := ao.sortStreamsByActivity(streams)

		assert.Empty(t, result)
	})

	t.Run("1要素", func(t *testing.T) {
		streams := []*AudioStream{
			{ID: "id1", AccumulatedLevel: 100, ActiveCount: 10},
		}

		result := ao.sortStreamsByActivity(streams)

		require.Len(t, result, 1)
		assert.Equal(t, "id1", result[0].ID)
	})
}

func TestAudioObserver_ChangedStreamIDs(t *testing.T) {
	ao := NewAudioObserver(50, 100, 50)

	t.Run("長さが異なる場合は変更あり", func(t *testing.T) {
		previous := []string{"a", "b"}
		current := []string{"a", "b", "c"}

		result := ao.changedStreamIDs(previous, current)

		assert.Equal(t, current, result)
	})

	t.Run("同じ内容の場合は変更なし", func(t *testing.T) {
		previous := []string{"a", "b", "c"}
		current := []string{"a", "b", "c"}

		result := ao.changedStreamIDs(previous, current)

		assert.Nil(t, result)
	})

	t.Run("順序が異なる場合は変更あり", func(t *testing.T) {
		previous := []string{"a", "b", "c"}
		current := []string{"c", "b", "a"}

		result := ao.changedStreamIDs(previous, current)

		assert.Equal(t, current, result)
	})

	t.Run("部分的に異なる場合は変更あり", func(t *testing.T) {
		previous := []string{"a", "b", "c"}
		current := []string{"a", "x", "c"}

		result := ao.changedStreamIDs(previous, current)

		assert.Equal(t, current, result)
	})

	t.Run("両方空の場合は変更なし", func(t *testing.T) {
		previous := []string{}
		current := []string{}

		result := ao.changedStreamIDs(previous, current)

		assert.Nil(t, result)
	})

	t.Run("previousが空でcurrentがある場合", func(t *testing.T) {
		previous := []string{}
		current := []string{"a"}

		result := ao.changedStreamIDs(previous, current)

		assert.Equal(t, current, result)
	})
}

func TestAudioObserver_Calc(t *testing.T) {
	t.Run("アクティブなストリームを返す", func(t *testing.T) {
		// expectedCount = 100 * 50 / 2000 = 2
		ao := NewAudioObserver(50, 100, 50)
		ao.addStream("stream1")
		ao.addStream("stream2")

		// stream1: 3回observe（expectedCount=2を超える）
		ao.observe("stream1", 10)
		ao.observe("stream1", 20)
		ao.observe("stream1", 30)

		// stream2: 1回observe（expectedCount=2を下回る）
		ao.observe("stream2", 10)

		result := ao.Calc()

		require.Len(t, result, 1)
		assert.Equal(t, "stream1", result[0])
	})

	t.Run("Calc後にカウンターがリセットされる", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)
		ao.addStream("stream1")

		ao.observe("stream1", 10)
		ao.observe("stream1", 20)
		ao.observe("stream1", 30)

		ao.Calc()

		// リセットされていることを確認
		assert.Equal(t, 0, ao.streams[0].AccumulatedLevel)
		assert.Equal(t, 0, ao.streams[0].ActiveCount)
	})

	t.Run("変更がない場合はnilを返す", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)
		ao.addStream("stream1")

		ao.observe("stream1", 10)
		ao.observe("stream1", 20)
		ao.observe("stream1", 30)

		// 1回目: 結果を返す
		result1 := ao.Calc()
		require.Len(t, result1, 1)

		// 同じ状態を再度作成
		ao.observe("stream1", 10)
		ao.observe("stream1", 20)
		ao.observe("stream1", 30)

		// 2回目: 変更がないのでnilを返す
		result2 := ao.Calc()
		assert.Nil(t, result2)
	})

	t.Run("アクティブストリームが変わった場合", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)
		ao.addStream("stream1")
		ao.addStream("stream2")

		// 1回目: stream1がアクティブ
		ao.observe("stream1", 10)
		ao.observe("stream1", 20)
		ao.observe("stream1", 30)

		result1 := ao.Calc()
		require.Len(t, result1, 1)
		assert.Equal(t, "stream1", result1[0])

		// 2回目: stream2がアクティブ
		ao.observe("stream2", 10)
		ao.observe("stream2", 20)
		ao.observe("stream2", 30)

		result2 := ao.Calc()
		require.Len(t, result2, 1)
		assert.Equal(t, "stream2", result2[0])
	})

	t.Run("複数のアクティブストリーム", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)
		ao.addStream("stream1")
		ao.addStream("stream2")
		ao.addStream("stream3")

		// stream1: count=5, level=50
		for i := 0; i < 5; i++ {
			ao.observe("stream1", 10)
		}
		// stream2: count=3, level=60
		for i := 0; i < 3; i++ {
			ao.observe("stream2", 20)
		}
		// stream3: count=1, level=30（expectedCount未満）
		ao.observe("stream3", 30)

		result := ao.Calc()

		require.Len(t, result, 2)
		// ActiveCount降順でソートされる
		assert.Equal(t, "stream1", result[0]) // count=5
		assert.Equal(t, "stream2", result[1]) // count=3
	})

	t.Run("ストリームがない場合", func(t *testing.T) {
		ao := NewAudioObserver(50, 100, 50)

		result := ao.Calc()

		assert.Empty(t, result)
	})

	t.Run("expectedCount=0の場合（全ストリームがアクティブ）", func(t *testing.T) {
		// filter=0 なので expectedCount=0
		ao := NewAudioObserver(50, 100, 0)
		ao.addStream("stream1")
		ao.addStream("stream2")

		// 0回でもexpectedCount(0)以上
		result := ao.Calc()

		require.Len(t, result, 2)
	})
}

func TestAudioObserver_ConcurrentAccess(t *testing.T) {
	ao := NewAudioObserver(50, 100, 50)

	var wg sync.WaitGroup

	// 並行してストリームを追加
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ao.addStream("stream" + string(rune('0'+id)))
		}(i)
	}

	wg.Wait()

	// 並行してobserve
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ao.observe("stream"+string(rune('0'+idx%10)), uint8(idx%50))
		}(i)
	}

	wg.Wait()

	// 並行してCalc
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ao.Calc()
		}()
	}

	wg.Wait()

	// パニックなく完了すればテスト成功
	assert.Len(t, ao.streams, 10)
}

package parallel

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestMapPreservesOrder(t *testing.T) {
	items := []int{1, 2, 3, 4}

	got := Map(items, func(_ int, item int) int {
		return item * 10
	})

	want := []int{10, 20, 30, 40}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("result[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestMapLimitBoundsConcurrency(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6}
	var active atomic.Int32
	var maxActive atomic.Int32

	MapLimit(items, 2, func(_ int, item int) int {
		now := active.Add(1)
		for {
			prev := maxActive.Load()
			if now <= prev || maxActive.CompareAndSwap(prev, now) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		active.Add(-1)
		return item
	})

	if got := maxActive.Load(); got > 2 {
		t.Fatalf("max active workers = %d, want <= 2", got)
	}
}

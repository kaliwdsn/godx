package memorymanager

import (
	"testing"
	"time"
)

func TestMemoryManager_SetMemoryLimit_Expand(t *testing.T) {
	done := make(chan struct{}, 1)
	mm := New(100000, stopChan)

	mm.Request(150000, true)
	if mm.underflow != 50000 {
		t.Errorf("error: expected underflow to be 50000, got %d", mm.underflow)
	}

	go func() {
		mm.Request(5000, false)
		done <- struct{}{}
	}()

	mm.SetMemoryLimit(156000)

	select {
	case <-done:
		if mm.limit != 156000 {
			t.Errorf("the memory limit is expanded to 106000, instead, got %d", mm.limit)
		}
		if mm.underflow != 0 {
			t.Errorf("the memory underflow is expected to be 0, instead, got %d", mm.underflow)
		}
		if mm.available != 1000 {
			t.Errorf("the memory left is expected to be 1000, instead, got %d", mm.available)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("error: memory request is expected to be successfully, limit expanded")
	}
}

func TestMemoryManager_SetMemoryLimit_Shrink(t *testing.T) {
	mm := New(10000, stopChan)
	mm.Request(15000, true)
	mm.SetMemoryLimit(5000)
	if mm.underflow != 10000 {
		t.Errorf("error: memoery shrinked, the memory underflow should be 10000, instead got: %d",
			mm.underflow)
	}

	if mm.limit != 5000 {
		t.Errorf("error: memory shrinked, the limit should be 5000, instead got: %d", mm.limit)
	}
	mm.Return(15000)
	if mm.available != 5000 {
		t.Errorf("error: memory shinked, memory left should be 5000, instead got: %d", mm.available)
	}
}
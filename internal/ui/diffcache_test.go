package ui

import "testing"

func TestDiffBodyCacheEvictsOldestPR(t *testing.T) {
	c := newDiffBodyCache(2)
	c.Set(1, "k1", []byte("a"))
	c.Set(2, "k2", []byte("b"))
	c.Set(3, "k3", []byte("c")) // evicts pr 1

	if _, ok := c.Get("k1"); ok {
		t.Errorf("pr 1 key should have been evicted")
	}
	if b, ok := c.Get("k2"); !ok || string(b) != "b" {
		t.Errorf("pr 2 key missing")
	}
	if b, ok := c.Get("k3"); !ok || string(b) != "c" {
		t.Errorf("pr 3 key missing")
	}
}

func TestDiffBodyCacheReserveBlocksDuplicateFetch(t *testing.T) {
	c := newDiffBodyCache(5)
	c.Reserve(1, "k")
	if b, ok := c.Get("k"); !ok || b != nil {
		t.Errorf("Reserve should make Get return (nil,true); got body=%v ok=%v", b, ok)
	}
	c.Set(1, "k", []byte("done"))
	if b, _ := c.Get("k"); string(b) != "done" {
		t.Errorf("Set should overwrite Reserve; got %q", b)
	}
}

func TestDiffBodyCacheClearPRReleasesSlot(t *testing.T) {
	c := newDiffBodyCache(2)
	c.Set(1, "k1", []byte("a"))
	c.Set(2, "k2", []byte("b"))
	c.ClearPR(1)
	c.Set(3, "k3", []byte("c")) // should NOT evict pr 2
	if _, ok := c.Get("k2"); !ok {
		t.Errorf("pr 2 should still be cached after ClearPR(1) freed a slot")
	}
}

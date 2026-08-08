package docker

import (
	"testing"
	"time"
)

func TestInspectCacheInvalidationDropsInFlightPut(t *testing.T) {
	c := &Client{}
	gen := c.inspectGen("lab")
	c.putInspectCache("lab", InspectInfo{Name: "lab", Exists: true, Running: true}, gen)
	if _, ok := c.getInspectCache("lab"); !ok {
		t.Fatal("expected cache hit")
	}
	c.InvalidateInspect("lab")
	if _, ok := c.getInspectCache("lab"); ok {
		t.Fatal("expected miss after invalidate")
	}
	// Stale put with old generation must not revive the entry.
	c.putInspectCache("lab", InspectInfo{Name: "lab", Exists: true, Running: false}, gen)
	if _, ok := c.getInspectCache("lab"); ok {
		t.Fatal("stale put after invalidate must be ignored")
	}
	// Fresh put with current generation works.
	gen2 := c.inspectGen("lab")
	c.putInspectCache("lab", InspectInfo{Name: "lab", Exists: true, Status: "running", Running: true}, gen2)
	info, ok := c.getInspectCache("lab")
	if !ok || !info.Running {
		t.Fatalf("expected fresh cache hit, ok=%v info=%+v", ok, info)
	}
	// Expiry
	c.inspectCache.Store("lab", inspectCacheEntry{
		info:    info,
		expires: time.Now().Add(-time.Second),
		gen:     gen2,
	})
	if _, ok := c.getInspectCache("lab"); ok {
		t.Fatal("expected miss after expiry")
	}
}

package status

import (
	"sort"
	"sync"
	"testing"
)

func TestNewTracker(t *testing.T) {
	tr := NewTracker()
	if tr == nil {
		t.Fatal("NewTracker returned nil")
	}
	all := tr.All()
	if len(all) != 0 {
		t.Fatalf("expected empty All(), got %d items", len(all))
	}
}

func TestSetAndAll(t *testing.T) {
	tr := NewTracker()

	tr.Set("cluster-a", Connecting)
	tr.Set("cluster-b", Connected)
	tr.Set("cluster-c", Disconnected)

	all := tr.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 clusters, got %d", len(all))
	}

	m := make(map[string]ClusterStatus)
	for _, cs := range all {
		m[cs.Name] = cs
	}

	if m["cluster-a"].Status != Connecting {
		t.Errorf("cluster-a: expected %s, got %s", Connecting, m["cluster-a"].Status)
	}
	if m["cluster-b"].Status != Connected {
		t.Errorf("cluster-b: expected %s, got %s", Connected, m["cluster-b"].Status)
	}
	if m["cluster-c"].Status != Disconnected {
		t.Errorf("cluster-c: expected %s, got %s", Disconnected, m["cluster-c"].Status)
	}
}

func TestSetOverwritesState(t *testing.T) {
	tr := NewTracker()
	tr.Set("cluster-a", Connecting)
	tr.Set("cluster-a", Connected)

	all := tr.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(all))
	}
	if all[0].Status != Connected {
		t.Errorf("expected %s, got %s", Connected, all[0].Status)
	}
}

func TestSetPausedAndIsPaused(t *testing.T) {
	tr := NewTracker()
	tr.Set("cluster-a", Connected)

	if tr.IsPaused("cluster-a") {
		t.Error("expected cluster-a not paused initially")
	}

	tr.SetPaused("cluster-a", true)
	if !tr.IsPaused("cluster-a") {
		t.Error("expected cluster-a paused after SetPaused(true)")
	}

	tr.SetPaused("cluster-a", false)
	if tr.IsPaused("cluster-a") {
		t.Error("expected cluster-a not paused after SetPaused(false)")
	}
}

func TestIsPausedUnknownCluster(t *testing.T) {
	tr := NewTracker()
	if tr.IsPaused("nonexistent") {
		t.Error("expected false for unknown cluster")
	}
}

func TestConfiguredClusterNames(t *testing.T) {
	tr := NewTracker()

	names := tr.ConfiguredClusterNames()
	if len(names) != 0 {
		t.Fatalf("expected empty names, got %d", len(names))
	}

	tr.Set("beta", Connecting)
	tr.Set("alpha", Connected)
	tr.Set("gamma", Disconnected)

	names = tr.ConfiguredClusterNames()
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}

	sort.Strings(names)
	expected := []string{"alpha", "beta", "gamma"}
	for i, e := range expected {
		if names[i] != e {
			t.Errorf("index %d: expected %s, got %s", i, e, names[i])
		}
	}
}

func TestAllReturnsPausedFlag(t *testing.T) {
	tr := NewTracker()
	tr.Set("cluster-a", Connected)
	tr.Set("cluster-b", Connected)
	tr.SetPaused("cluster-a", true)

	all := tr.All()
	m := make(map[string]ClusterStatus)
	for _, cs := range all {
		m[cs.Name] = cs
	}

	if !m["cluster-a"].Paused {
		t.Error("expected cluster-a paused=true in All()")
	}
	if m["cluster-b"].Paused {
		t.Error("expected cluster-b paused=false in All()")
	}
}

func TestConcurrentAccess(t *testing.T) {
	tr := NewTracker()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(3)
		name := "cluster"
		go func() {
			defer wg.Done()
			tr.Set(name, Connected)
		}()
		go func() {
			defer wg.Done()
			tr.SetPaused(name, true)
		}()
		go func() {
			defer wg.Done()
			_ = tr.All()
		}()
	}

	wg.Wait()

	all := tr.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 cluster after concurrent access, got %d", len(all))
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	tr := NewTracker()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(4)
		go func(n int) {
			defer wg.Done()
			tr.Set("c1", Connected)
		}(i)
		go func(n int) {
			defer wg.Done()
			tr.Set("c2", Disconnected)
		}(i)
		go func(n int) {
			defer wg.Done()
			_ = tr.IsPaused("c1")
		}(i)
		go func(n int) {
			defer wg.Done()
			_ = tr.ConfiguredClusterNames()
		}(i)
	}

	wg.Wait()
}

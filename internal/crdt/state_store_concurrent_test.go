package crdt

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLWWMap_ConcurrentSetAndGet(t *testing.T) {
	m := NewLWWMap[string]()
	clock := NewHLC("peer-a")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i%10)
			m.Set(key, fmt.Sprintf("val-%d", i), clock.Now())
			m.Get(key)
		}(i)
	}
	wg.Wait()

	// All 10 keys should exist
	assert.Equal(t, 10, m.Len())
}

func TestLWWMap_ConcurrentMerge(t *testing.T) {
	m1 := NewLWWMap[string]()
	clock1 := NewHLC("peer-a")

	m2 := NewLWWMap[string]()
	clock2 := NewHLC("peer-b")

	// Populate both maps concurrently
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			m1.Set(fmt.Sprintf("k%d", i), "a", clock1.Now())
		}(i)
		go func(i int) {
			defer wg.Done()
			m2.Set(fmt.Sprintf("k%d", i), "b", clock2.Now())
		}(i)
	}
	wg.Wait()

	// Merge concurrently from multiple goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m1.Merge(m2)
		}()
	}
	wg.Wait()

	// Should have all 50 keys, no panics, no races
	assert.Equal(t, 50, m1.Len())
}

func TestStateStore_ConcurrentOperations(t *testing.T) {
	store := NewStateStore("peer-a")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			store.SetPeer(fmt.Sprintf("p%d", i%5), PeerInfo{Name: fmt.Sprintf("peer-%d", i)})
		}(i)
		go func(i int) {
			defer wg.Done()
			store.SetConfig(fmt.Sprintf("c%d", i%5), fmt.Sprintf("val-%d", i))
		}(i)
		go func(i int) {
			defer wg.Done()
			store.SetRoute(fmt.Sprintf("r%d", i%5), RouteRule{
				Match: RouteMatch{Project: fmt.Sprintf("proj-%d", i)},
			})
		}(i)
	}
	wg.Wait()

	// 5 unique keys each
	assert.Equal(t, 5, len(store.ListPeers()))
	assert.Equal(t, 5, len(store.ListRoutes()))
}

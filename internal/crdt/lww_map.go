package crdt

import (
	"encoding/json"
	"sort"
	"sync"
)

// LWWMap is a map of string keys to LWW-Registers.
// Safe for concurrent use.
type LWWMap[T any] struct {
	mu      sync.RWMutex
	entries map[string]*LWWRegister[T]
}

// NewLWWMap creates a new empty LWW-Map.
func NewLWWMap[T any]() *LWWMap[T] {
	return &LWWMap[T]{
		entries: make(map[string]*LWWRegister[T]),
	}
}

// Set updates or inserts a key-value pair with the given timestamp.
func (m *LWWMap[T]) Set(key string, value T, ts Timestamp) {
	m.mu.Lock()
	defer m.mu.Unlock()

	reg, ok := m.entries[key]
	if !ok {
		reg = NewLWWRegister[T]()
		m.entries[key] = reg
	}
	reg.Set(value, ts)
}

// Delete marks a key as deleted with the given timestamp.
func (m *LWWMap[T]) Delete(key string, ts Timestamp) {
	m.mu.Lock()
	defer m.mu.Unlock()

	reg, ok := m.entries[key]
	if !ok {
		reg = NewLWWRegister[T]()
		m.entries[key] = reg
	}
	reg.Delete(ts)
}

// Get returns the value for a key if it exists and is not deleted.
func (m *LWWMap[T]) Get(key string) (T, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	reg, ok := m.entries[key]
	if !ok {
		var zero T
		return zero, false
	}
	return reg.Get()
}

// Keys returns all non-deleted keys, sorted.
func (m *LWWMap[T]) Keys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0)
	for k, reg := range m.entries {
		if v, ok := reg.Get(); ok {
			_ = v
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// Len returns the number of non-deleted entries.
func (m *LWWMap[T]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, reg := range m.entries {
		if _, ok := reg.Get(); ok {
			count++
		}
	}
	return count
}

// Merge incorporates all entries from another map.
// Per-key LWW semantics: the register with the higher timestamp wins.
func (m *LWWMap[T]) Merge(other *LWWMap[T]) {
	if other == nil {
		return
	}

	other.mu.RLock()
	otherEntries := make(map[string]*LWWRegister[T], len(other.entries))
	for k, v := range other.entries {
		otherEntries[k] = v
	}
	other.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	for key, otherReg := range otherEntries {
		localReg, ok := m.entries[key]
		if !ok {
			clone := NewLWWRegister[T]()
			clone.Merge(otherReg)
			m.entries[key] = clone
		} else {
			localReg.Merge(otherReg)
		}
	}
}

// Delta returns a new LWWMap containing only entries with timestamps
// strictly after the given timestamp.
func (m *LWWMap[T]) Delta(since Timestamp) *LWWMap[T] {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := NewLWWMap[T]()
	for key, reg := range m.entries {
		if reg.IsSet() && reg.TS.After(since) {
			clone := NewLWWRegister[T]()
			clone.Merge(reg)
			result.entries[key] = clone
		}
	}
	return result
}

// Snapshot returns a deep copy of the entire map.
func (m *LWWMap[T]) Snapshot() *LWWMap[T] {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := NewLWWMap[T]()
	for key, reg := range m.entries {
		clone := NewLWWRegister[T]()
		clone.Merge(reg)
		result.entries[key] = clone
	}
	return result
}

// MarshalJSON implements json.Marshaler.
func (m *LWWMap[T]) MarshalJSON() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return json.Marshal(m.entries)
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *LWWMap[T]) UnmarshalJSON(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries := make(map[string]*LWWRegister[T])
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	m.entries = entries
	return nil
}

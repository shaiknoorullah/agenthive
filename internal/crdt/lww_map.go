package crdt

// LWWMap is a map of string keys to LWW-Registers.
type LWWMap[T any] struct{}

func NewLWWMap[T any]() *LWWMap[T]                               { return nil }
func (m *LWWMap[T]) Set(key string, value T, ts Timestamp)       {}
func (m *LWWMap[T]) Delete(key string, ts Timestamp)              {}
func (m *LWWMap[T]) Get(key string) (T, bool)                     { var zero T; return zero, false }
func (m *LWWMap[T]) Keys() []string                               { return nil }
func (m *LWWMap[T]) Len() int                                     { return 0 }
func (m *LWWMap[T]) Merge(other *LWWMap[T])                       {}
func (m *LWWMap[T]) Delta(since Timestamp) *LWWMap[T]             { return nil }
func (m *LWWMap[T]) Snapshot() *LWWMap[T]                         { return nil }

package crdt

// LWWRegister is a Last-Writer-Wins Register.
type LWWRegister[T any] struct{}

func NewLWWRegister[T any]() *LWWRegister[T]            { return nil }
func (r *LWWRegister[T]) Set(value T, ts Timestamp)     {}
func (r *LWWRegister[T]) Delete(ts Timestamp)            {}
func (r *LWWRegister[T]) Get() (T, bool)                 { var zero T; return zero, false }
func (r *LWWRegister[T]) IsDeleted() bool                { return false }
func (r *LWWRegister[T]) Merge(other *LWWRegister[T])    {}
func (r *LWWRegister[T]) GetTimestamp() Timestamp        { return Timestamp{} }
func (r *LWWRegister[T]) IsSet() bool                    { return false }

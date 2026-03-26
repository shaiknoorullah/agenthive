package crdt

import "encoding/json"

// LWWRegister is a Last-Writer-Wins Register.
// The value with the highest timestamp wins on merge.
// Supports soft deletion via a deleted flag.
type LWWRegister[T any] struct {
	Value   T         `json:"value"`
	TS      Timestamp `json:"ts"`
	Deleted bool      `json:"deleted,omitempty"`
	set     bool      // tracks whether any value has been set
}

// NewLWWRegister creates a new empty LWW-Register.
func NewLWWRegister[T any]() *LWWRegister[T] {
	return &LWWRegister[T]{}
}

// Set updates the register value if ts is after the current timestamp.
func (r *LWWRegister[T]) Set(value T, ts Timestamp) {
	if !r.set || ts.After(r.TS) {
		r.Value = value
		r.TS = ts
		r.Deleted = false
		r.set = true
	}
}

// Delete marks the register as deleted if ts is after the current timestamp.
func (r *LWWRegister[T]) Delete(ts Timestamp) {
	if !r.set || ts.After(r.TS) {
		r.TS = ts
		r.Deleted = true
		r.set = true
	}
}

// Get returns the value and whether it is set (and not deleted).
func (r *LWWRegister[T]) Get() (T, bool) {
	if !r.set || r.Deleted {
		var zero T
		return zero, false
	}
	return r.Value, true
}

// IsDeleted returns whether the register has been soft-deleted.
func (r *LWWRegister[T]) IsDeleted() bool {
	return r.set && r.Deleted
}

// Timestamp returns the current timestamp of the register.
func (r *LWWRegister[T]) GetTimestamp() Timestamp {
	return r.TS
}

// IsSet returns whether the register has been initialized.
func (r *LWWRegister[T]) IsSet() bool {
	return r.set
}

// Merge incorporates the state from another register.
// The register with the higher timestamp wins.
func (r *LWWRegister[T]) Merge(other *LWWRegister[T]) {
	if other == nil || !other.set {
		return
	}
	if !r.set || other.TS.After(r.TS) {
		r.Value = other.Value
		r.TS = other.TS
		r.Deleted = other.Deleted
		r.set = true
	}
}

// MarshalJSON implements json.Marshaler.
func (r *LWWRegister[T]) MarshalJSON() ([]byte, error) {
	type alias struct {
		Value   T         `json:"value"`
		TS      Timestamp `json:"ts"`
		Deleted bool      `json:"deleted,omitempty"`
	}
	return json.Marshal(alias{Value: r.Value, TS: r.TS, Deleted: r.Deleted})
}

// UnmarshalJSON implements json.Unmarshaler.
func (r *LWWRegister[T]) UnmarshalJSON(data []byte) error {
	type alias struct {
		Value   T         `json:"value"`
		TS      Timestamp `json:"ts"`
		Deleted bool      `json:"deleted,omitempty"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	r.Value = a.Value
	r.TS = a.TS
	r.Deleted = a.Deleted
	r.set = true
	return nil
}

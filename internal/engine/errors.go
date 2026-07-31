// Package engine typed errors replace the Rust MoaError enum.
package engine

import (
	"errors"
	"fmt"
)

// ErrNoReferences indicates the config has no reference models.
var ErrNoReferences = errors.New("At least one reference model is required, but none are configured")

// ConfigValidationError wraps a validation message from the config package.
type ConfigValidationError struct {
	Msg string
}

func (e *ConfigValidationError) Error() string {
	return fmt.Sprintf("Config validation failed: %s", e.Msg)
}

// ReferenceFailedError records a single reference model failure.
type ReferenceFailedError struct {
	Label string
	Model string
	Err   error
}

func (e *ReferenceFailedError) Error() string {
	return fmt.Sprintf("Reference model %s (%s) failed: %v", e.Label, e.Model, e.Err)
}

func (e *ReferenceFailedError) Unwrap() error { return e.Err }

// AggregatorFailedError wraps an aggregator backend failure.
type AggregatorFailedError struct {
	Err error
}

func (e *AggregatorFailedError) Error() string {
	return fmt.Sprintf("Aggregator run failed: %v", e.Err)
}

func (e *AggregatorFailedError) Unwrap() error { return e.Err }

// OutputWriteError wraps a filesystem error while writing output.
type OutputWriteError struct {
	Err error
}

func (e *OutputWriteError) Error() string {
	return fmt.Sprintf("Output write failed: %v", e.Err)
}

func (e *OutputWriteError) Unwrap() error { return e.Err }

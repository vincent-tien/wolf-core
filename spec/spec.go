// Package spec provides a composable Specification pattern for domain rules.
// Specifications encapsulate business predicates that can be combined with
// And/Or/Not operators and checked against entities to produce structured
// violation errors with spec names for traceability.
package spec

import (
	"fmt"
	"strings"
)

// Spec defines a named business rule predicate over type T.
type Spec[T any] interface {
	Name() string
	IsSatisfiedBy(entity T) bool
}

// Violation is returned when a specification is not satisfied.
type Violation struct {
	SpecName string
	Message  string
}

func (v *Violation) Error() string {
	return fmt.Sprintf("specification %q violated: %s", v.SpecName, v.Message)
}

// Check evaluates spec against entity and returns a *Violation error if not
// satisfied, or nil on success.
func Check[T any](spec Spec[T], entity T) error {
	if spec.IsSatisfiedBy(entity) {
		return nil
	}
	return &Violation{
		SpecName: spec.Name(),
		Message:  fmt.Sprintf("%s is not satisfied", spec.Name()),
	}
}

// CheckAll evaluates all specs against entity and collects every violation.
// Returns nil when all specs pass.
func CheckAll[T any](entity T, specs ...Spec[T]) []error {
	var errs []error
	for _, s := range specs {
		if err := Check(s, entity); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// New creates a named Spec from a predicate function.
func New[T any](name string, pred func(T) bool) Spec[T] {
	return &funcSpec[T]{name: name, pred: pred}
}

type funcSpec[T any] struct {
	name string
	pred func(T) bool
}

func (s *funcSpec[T]) Name() string              { return s.name }
func (s *funcSpec[T]) IsSatisfiedBy(e T) bool     { return s.pred(e) }

// And returns a composite spec that requires both specs to be satisfied.
func And[T any](left, right Spec[T]) Spec[T] {
	return &andSpec[T]{left: left, right: right}
}

type andSpec[T any] struct {
	left, right Spec[T]
}

func (s *andSpec[T]) Name() string {
	return s.left.Name() + " AND " + s.right.Name()
}

func (s *andSpec[T]) IsSatisfiedBy(e T) bool {
	return s.left.IsSatisfiedBy(e) && s.right.IsSatisfiedBy(e)
}

// Or returns a composite spec satisfied when either spec passes.
func Or[T any](left, right Spec[T]) Spec[T] {
	return &orSpec[T]{left: left, right: right}
}

type orSpec[T any] struct {
	left, right Spec[T]
}

func (s *orSpec[T]) Name() string {
	return s.left.Name() + " OR " + s.right.Name()
}

func (s *orSpec[T]) IsSatisfiedBy(e T) bool {
	return s.left.IsSatisfiedBy(e) || s.right.IsSatisfiedBy(e)
}

// Not returns a spec satisfied when the inner spec is NOT satisfied.
func Not[T any](inner Spec[T]) Spec[T] {
	return &notSpec[T]{inner: inner}
}

type notSpec[T any] struct {
	inner Spec[T]
}

func (s *notSpec[T]) Name() string {
	return "NOT " + s.inner.Name()
}

func (s *notSpec[T]) IsSatisfiedBy(e T) bool {
	return !s.inner.IsSatisfiedBy(e)
}

// FormatViolations joins a slice of spec violation errors into a single
// human-readable string. Useful for error responses.
func FormatViolations(errs []error) string {
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

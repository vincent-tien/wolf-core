// capture.go — Defer-safe error capture for Close calls (errcapture pattern).
package errors

import "errors"

// Do calls closer and merges any returned error into *err using errors.Join.
// It is intended for use in deferred Close calls where the caller wants to
// capture close failures without discarding the original function error:
//
//	func query(ctx context.Context) (_ []Item, err error) {
//	    rows, err := db.QueryContext(ctx, ...)
//	    if err != nil { return nil, err }
//	    defer errcapture.Do(&err, rows.Close, "close rows")
//	    ...
//	}
//
// If closer returns nil, *err is unchanged. If both *err and the close error
// are non-nil, they are joined so that errors.Is/As works on either cause.
func Do(err *error, closer func() error, msg string) {
	closeErr := closer()
	if closeErr == nil {
		return
	}
	closeErr = Wrap(closeErr, msg)
	*err = errors.Join(*err, closeErr)
}

// Wrap returns a new error that wraps cause with the given message prefix.
// If cause is nil, Wrap returns nil.
func Wrap(cause error, msg string) error {
	if cause == nil {
		return nil
	}
	return &wrappedError{msg: msg, cause: cause}
}

type wrappedError struct {
	msg   string
	cause error
}

func (e *wrappedError) Error() string { return e.msg + ": " + e.cause.Error() }
func (e *wrappedError) Unwrap() error { return e.cause }

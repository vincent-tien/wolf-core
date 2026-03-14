// Package io provides streaming I/O utilities for bounded-memory processing.
package io

import (
	"bufio"
	"io"
)

const defaultBufSize = 64 * 1024 // 64 KiB

// StreamProcess reads lines from r using a single reusable buffer capped at
// bufSize bytes. Each complete line (without the trailing newline) is passed
// to processLine. Processing stops on the first error from processLine or
// when the reader is exhausted.
//
// When bufSize <= 0, it defaults to 64 KiB. Lines exceeding bufSize cause
// bufio.ErrTooLong to be returned.
func StreamProcess(r io.Reader, bufSize int, processLine func(line []byte) error) error {
	if bufSize <= 0 {
		bufSize = defaultBufSize
	}

	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, bufSize)
	scanner.Buffer(buf, bufSize)

	for scanner.Scan() {
		if err := processLine(scanner.Bytes()); err != nil {
			return err
		}
	}

	return scanner.Err()
}

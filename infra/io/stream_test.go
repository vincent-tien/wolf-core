package io_test

import (
	"bufio"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pio "github.com/vincent-tien/wolf-core/infra/io"
)

func TestStreamProcess_ProcessesAllLines(t *testing.T) {
	t.Parallel()

	// Arrange
	input := "line1\nline2\nline3\n"
	var count int

	// Act
	err := pio.StreamProcess(strings.NewReader(input), 0, func(line []byte) error {
		count++
		return nil
	})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestStreamProcess_EmptyReader(t *testing.T) {
	t.Parallel()

	// Arrange
	var called bool

	// Act
	err := pio.StreamProcess(strings.NewReader(""), 0, func(line []byte) error {
		called = true
		return nil
	})

	// Assert
	require.NoError(t, err)
	assert.False(t, called)
}

func TestStreamProcess_ErrorStopsEarly(t *testing.T) {
	t.Parallel()

	// Arrange
	input := "a\nb\nc\n"
	stopErr := errors.New("stop")
	var count int

	// Act
	err := pio.StreamProcess(strings.NewReader(input), 0, func(line []byte) error {
		count++
		if count == 2 {
			return stopErr
		}
		return nil
	})

	// Assert
	assert.ErrorIs(t, err, stopErr)
	assert.Equal(t, 2, count)
}

func TestStreamProcess_DefaultBufSize(t *testing.T) {
	t.Parallel()

	// Arrange — a line well under 64 KiB should work with bufSize=0
	input := strings.Repeat("x", 1000) + "\n"
	var received int

	// Act
	err := pio.StreamProcess(strings.NewReader(input), 0, func(line []byte) error {
		received = len(line)
		return nil
	})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1000, received)
}

func TestStreamProcess_LineLongerThanBuffer(t *testing.T) {
	t.Parallel()

	// Arrange — 256-byte line with only 64-byte buffer
	input := strings.Repeat("x", 256) + "\n"

	// Act
	err := pio.StreamProcess(strings.NewReader(input), 64, func(line []byte) error {
		return nil
	})

	// Assert
	assert.ErrorIs(t, err, bufio.ErrTooLong)
}

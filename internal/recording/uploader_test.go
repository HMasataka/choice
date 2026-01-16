package recording

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormatISO8601Duration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "seconds only",
			duration: 30 * time.Second,
			expected: "PT30S",
		},
		{
			name:     "minutes and seconds",
			duration: 5*time.Minute + 30*time.Second,
			expected: "PT5M30S",
		},
		{
			name:     "hours, minutes, and seconds",
			duration: 2*time.Hour + 15*time.Minute + 45*time.Second,
			expected: "PT2H15M45S",
		},
		{
			name:     "zero duration",
			duration: 0,
			expected: "PT0S",
		},
		{
			name:     "one hour exactly",
			duration: time.Hour,
			expected: "PT1H0M0S",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatISO8601Duration(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBytesReader(t *testing.T) {
	data := []byte("hello world")
	reader := &bytesReader{data: data}

	// Read in chunks
	buf := make([]byte, 5)
	n, err := reader.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(buf))

	// Read remaining
	buf = make([]byte, 10)
	n, err = reader.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 6, n)
	assert.Equal(t, " world", string(buf[:n]))

	// Read at EOF
	n, err = reader.Read(buf)
	assert.Error(t, err) // EOF
	assert.Equal(t, 0, n)
}

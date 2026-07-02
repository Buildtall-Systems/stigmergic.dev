package timeutil

import (
	"testing"
	"time"
)

func TestRelativeTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{
			name:     "under a minute",
			input:    now.Add(-30 * time.Second),
			expected: "just now",
		},
		{
			name:     "exactly one minute",
			input:    now.Add(-1 * time.Minute),
			expected: "1 minute ago",
		},
		{
			name:     "5 minutes ago",
			input:    now.Add(-5 * time.Minute),
			expected: "5 minutes ago",
		},
		{
			name:     "exactly one hour",
			input:    now.Add(-1 * time.Hour),
			expected: "1 hour ago",
		},
		{
			name:     "3 hours ago",
			input:    now.Add(-3 * time.Hour),
			expected: "3 hours ago",
		},
		{
			name:     "previous day",
			input:    now.Add(-30 * time.Hour),
			expected: "yesterday",
		},
		{
			name:     "3 days ago",
			input:    now.Add(-3 * 24 * time.Hour),
			expected: "3 days ago",
		},
		{
			name:     "same year date",
			input:    now.Add(-10 * 24 * time.Hour),
			expected: now.Add(-10 * 24 * time.Hour).Format("Jan 2"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RelativeTime(tt.input)
			if result != tt.expected {
				t.Errorf("RelativeTime() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestRelativeTimeDifferentYear(t *testing.T) {
	pastYear := time.Date(2023, 6, 15, 12, 0, 0, 0, time.Local)
	result := RelativeTime(pastYear)
	expected := "Jun 15, 2023"
	if result != expected {
		t.Errorf("RelativeTime() = %q, want %q", result, expected)
	}
}

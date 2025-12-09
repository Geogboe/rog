package cmd

import (
	"testing"
)

func TestExtractUnknownCommand(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected string
	}{
		{
			name:     "standard error message",
			errMsg:   `unknown command "foo" for "rog"`,
			expected: "foo",
		},
		{
			name:     "with newlines",
			errMsg:   "unknown command \"test\" for \"rog\"\nDid you mean this?\n\tlist",
			expected: "test",
		},
		{
			name:     "no quotes",
			errMsg:   "unknown command foo for rog",
			expected: "",
		},
		{
			name:     "empty string",
			errMsg:   "",
			expected: "",
		},
		{
			name:     "command with dashes",
			errMsg:   `unknown command "my-command" for "rog"`,
			expected: "my-command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractUnknownCommand(tt.errMsg)
			if got != tt.expected {
				t.Errorf("extractUnknownCommand(%q) = %q, want %q", tt.errMsg, got, tt.expected)
			}
		})
	}
}

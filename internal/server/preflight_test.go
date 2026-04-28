package server

import "testing"

func TestSameNormalizedUser(t *testing.T) {
	tests := []struct {
		name         string
		expected     string
		actual       string
		goos         string
		computerName string
		want         bool
	}{
		{
			name:     "exact match",
			expected: "alice",
			actual:   "alice",
			goos:     "linux",
			want:     true,
		},
		{
			name:         "windows machine account matches system",
			expected:     "buildhost$",
			actual:       "system",
			goos:         "windows",
			computerName: "BUILDHOST",
			want:         true,
		},
		{
			name:         "windows localsystem alias matches machine account",
			expected:     "localsystem",
			actual:       "buildhost$",
			goos:         "windows",
			computerName: "BUILDHOST",
			want:         true,
		},
		{
			name:         "windows different machine account does not match system",
			expected:     "otherbox$",
			actual:       "system",
			goos:         "windows",
			computerName: "BUILDHOST",
			want:         false,
		},
		{
			name:         "windows regular users do not match",
			expected:     "alice",
			actual:       "bob",
			goos:         "windows",
			computerName: "BUILDHOST",
			want:         false,
		},
		{
			name:         "non windows mismatch remains invalid",
			expected:     "buildhost$",
			actual:       "system",
			goos:         "linux",
			computerName: "BUILDHOST",
			want:         false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sameNormalizedUser(tc.expected, tc.actual, tc.goos, tc.computerName)
			if got != tc.want {
				t.Fatalf("sameNormalizedUser(%q, %q, %q, %q) = %v, want %v", tc.expected, tc.actual, tc.goos, tc.computerName, got, tc.want)
			}
		})
	}
}

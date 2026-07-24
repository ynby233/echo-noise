package middleware

import "testing"

func TestSuspiciousPathKeepsAPIBoundaryExact(t *testing.T) {
	tests := []struct {
		path       string
		suspicious bool
	}{
		{path: "/api", suspicious: false},
		{path: "/api/login", suspicious: false},
		{path: "/api.php", suspicious: true},
		{path: "/api-backup/config.jsp", suspicious: true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isSuspiciousPath(tt.path); got != tt.suspicious {
				t.Fatalf("isSuspiciousPath(%q) = %v, want %v", tt.path, got, tt.suspicious)
			}
		})
	}
}

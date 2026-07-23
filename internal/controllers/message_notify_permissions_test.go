package controllers

import "testing"

func boolPointer(value bool) *bool { return &value }

func TestShouldNotifyPublishedMessageRequiresAdmin(t *testing.T) {
	tests := []struct {
		name        string
		siteEnabled bool
		isAdmin     bool
		authVia     string
		requested   *bool
		want        bool
	}{
		{name: "admin session explicit true", siteEnabled: true, isAdmin: true, authVia: "session", requested: boolPointer(true), want: true},
		{name: "admin session explicit false", siteEnabled: true, isAdmin: true, authVia: "session", requested: boolPointer(false), want: false},
		{name: "admin token follows site switch", siteEnabled: true, isAdmin: true, authVia: "token", want: true},
		{name: "ordinary session cannot request push", siteEnabled: true, isAdmin: false, authVia: "session", requested: boolPointer(true), want: false},
		{name: "ordinary token cannot auto push", siteEnabled: true, isAdmin: false, authVia: "token", want: false},
		{name: "site switch disables admin push", siteEnabled: false, isAdmin: true, authVia: "session", requested: boolPointer(true), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldNotifyPublishedMessage(tt.siteEnabled, tt.isAdmin, tt.authVia, tt.requested); got != tt.want {
				t.Fatalf("shouldNotifyPublishedMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

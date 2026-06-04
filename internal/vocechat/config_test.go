package vocechat

import "testing"

func TestNormalizeAPIBaseURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: " ", want: ""},
		{name: "root", raw: "http://vc.example.com", want: "http://vc.example.com/api"},
		{name: "root trailing slash", raw: "http://vc.example.com/", want: "http://vc.example.com/api"},
		{name: "explicit api", raw: "http://vc.example.com/api/", want: "http://vc.example.com/api"},
		{name: "subpath", raw: "http://vc.example.com/voce", want: "http://vc.example.com/voce/api"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeAPIBaseURL(tc.raw); got != tc.want {
				t.Fatalf("NormalizeAPIBaseURL(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

package services

import "testing"

func TestDefaultHeaderImagesOnlyContainsSelectedImage(t *testing.T) {
	images := defaultHeaderImages()
	if len(images) != 1 {
		t.Fatalf("default header images count = %d, want 1", len(images))
	}
	if images[0] != defaultHeaderImageURL {
		t.Fatalf("default header image = %q, want %q", images[0], defaultHeaderImageURL)
	}
}

func TestShouldCollapseLegacyBackgrounds(t *testing.T) {
	tests := []struct {
		name        string
		backgrounds []string
		want        bool
	}{
		{
			name: "legacy defaults collapse",
			backgrounds: []string{
				"https://s2.loli.net/2025/03/27/KJ1trnU2ksbFEYM.jpg",
				defaultHeaderImageURL,
				"https://s2.loli.net/2025/03/27/y67m2k5xcSdTsHN.jpg",
			},
			want: true,
		},
		{
			name:        "selected default collapses",
			backgrounds: []string{" ", defaultHeaderImageURL},
			want:        true,
		},
		{
			name:        "custom image is preserved",
			backgrounds: []string{defaultHeaderImageURL, "https://example.com/custom.jpg"},
			want:        false,
		},
		{
			name:        "empty list is not legacy data",
			backgrounds: []string{},
			want:        false,
		},
		{
			name:        "blank list is not legacy data",
			backgrounds: []string{" ", ""},
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldCollapseLegacyBackgrounds(tt.backgrounds); got != tt.want {
				t.Fatalf("shouldCollapseLegacyBackgrounds() = %v, want %v", got, tt.want)
			}
		})
	}
}

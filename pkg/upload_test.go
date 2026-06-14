package pkg

import (
	"bytes"
	"strings"
	"testing"
)

func TestHashedAttachmentFileNameFromBytesIsStable(t *testing.T) {
	first := hashedAttachmentFileNameFromBytes([]byte("same file"), ".PNG")
	second := hashedAttachmentFileNameFromBytes([]byte("same file"), ".png")

	if first != second {
		t.Fatalf("expected stable filename, got %q and %q", first, second)
	}
	if !strings.HasSuffix(first, ".png") {
		t.Fatalf("expected normalized extension, got %q", first)
	}
}

func TestHashedAttachmentFileNameFromReadSeekerResetsReader(t *testing.T) {
	reader := bytes.NewReader([]byte("video bytes"))
	name, err := hashedAttachmentFileNameFromReadSeeker(reader, ".mp4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(name, ".mp4") {
		t.Fatalf("expected mp4 extension, got %q", name)
	}
	if pos, _ := reader.Seek(0, 1); pos != 0 {
		t.Fatalf("expected reader reset to start, got offset %d", pos)
	}
}

func TestIsAllowedImageTypeAcceptsImageWildcard(t *testing.T) {
	if !isAllowedImageType("image/avif", []string{"image/jpeg"}) {
		t.Fatal("expected image/* fallback to accept avif")
	}
	if isAllowedImageType("video/mp4", []string{"image/*"}) {
		t.Fatal("expected non-image mime type to be rejected")
	}
}

func TestIsAllowedTypeSupportsConfiguredWildcard(t *testing.T) {
	if !isAllowedType("image/svg+xml; charset=utf-8", []string{"image/*"}) {
		t.Fatal("expected configured wildcard to accept image subtype")
	}
}

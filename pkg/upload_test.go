package pkg

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttachmentContentHashFromBytesIsStable(t *testing.T) {
	first := attachmentContentHashFromBytes([]byte("same file"))
	second := attachmentContentHashFromBytes([]byte("same file"))

	if first != second {
		t.Fatalf("expected stable content hash, got %q and %q", first, second)
	}
	if strings.Contains(first, ".") {
		t.Fatalf("expected hash without filename extension, got %q", first)
	}
}

func TestAttachmentContentHashFromReadSeekerResetsReader(t *testing.T) {
	reader := bytes.NewReader([]byte("video bytes"))
	hash, err := attachmentContentHashFromReadSeeker(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash == "" {
		t.Fatal("expected content hash")
	}
	if pos, _ := reader.Seek(0, 1); pos != 0 {
		t.Fatalf("expected reader reset to start, got offset %d", pos)
	}
}

func TestSafeAttachmentFileNameKeepsOriginalStem(t *testing.T) {
	name := safeAttachmentFileName(`C:\uploads\假期照片.PNG`, ".png", "image")
	if name != "假期照片.png" {
		t.Fatalf("expected original stem with normalized extension, got %q", name)
	}
}

func TestLocalAttachmentFileNameForContentDeduplicatesExistingFile(t *testing.T) {
	dir := t.TempDir()
	content := []byte("same file")
	if err := os.WriteFile(filepath.Join(dir, "already-here.png"), content, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	name, existed, err := localAttachmentFileNameForContent(dir, "upload.png", attachmentContentHashFromBytes(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !existed || name != "already-here.png" {
		t.Fatalf("expected to reuse existing file, got name=%q existed=%v", name, existed)
	}
}

func TestLocalAttachmentFileNameForContentSequencesNameConflict(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "photo.png"), []byte("different file"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	name, existed, err := localAttachmentFileNameForContent(dir, "photo.png", attachmentContentHashFromBytes([]byte("new file")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if existed || name != "photo(1).png" {
		t.Fatalf("expected sequenced filename, got name=%q existed=%v", name, existed)
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

func TestIsAllowedTypeAcceptsAudioMimeParameters(t *testing.T) {
	allowed := []string{"audio/webm", "audio/ogg", "audio/mpeg", "audio/mp4", "audio/wav"}
	if !isAllowedType("audio/webm;codecs=opus", allowed) {
		t.Fatal("expected audio/webm with codec parameter to be accepted")
	}
	if isAllowedType("video/webm;codecs=vp9", allowed) {
		t.Fatal("expected video mime type to be rejected for audio upload")
	}
}

func TestAudioUploadExtAvoidsVideoMP4Classification(t *testing.T) {
	if ext := audioUploadExt("recording.mp4", "audio/mp4"); ext != ".m4a" {
		t.Fatalf("expected audio/mp4 to be stored as .m4a, got %q", ext)
	}
	if ext := audioUploadExt("recording.webm", "audio/webm;codecs=opus"); ext != ".webm" {
		t.Fatalf("expected webm extension to be preserved, got %q", ext)
	}
	if ext := audioUploadExt("recording.txt", "audio/mpeg"); ext != ".mp3" {
		t.Fatalf("expected audio/mpeg to use .mp3, got %q", ext)
	}
}

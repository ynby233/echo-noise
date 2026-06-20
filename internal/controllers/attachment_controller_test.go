package controllers

import "testing"

func TestIsAudioExt(t *testing.T) {
	accepted := []string{"recording.webm", "voice.ogg", "clip.mp3", "memo.m4a", "capture.wav"}
	for _, name := range accepted {
		if !isAudioExt(name) {
			t.Fatalf("expected %q to be treated as audio", name)
		}
	}

	rejected := []string{"photo.png", "movie.mov", "movie.mp4", "archive.zip", "audio"}
	for _, name := range rejected {
		if isAudioExt(name) {
			t.Fatalf("expected %q to be rejected as audio", name)
		}
	}
}
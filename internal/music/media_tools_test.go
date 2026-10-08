package music

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestActualToolsBoundCUESegmentAndALACRepresentation(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("actual codec checks require authorized environment FFmpeg; fixture test runs on fnOS if unavailable")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatal("ffmpeg present but ffprobe missing")
	}
	s, _ := musicFixture(t)
	s.opts.Tools = commandRunner{}
	s.opts.FFmpegPath = ffmpeg
	s.opts.FFprobePath = ffprobe
	s.toolsReady = false
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = s.checkTools(ctx); err != nil {
		t.Fatal("restricted tool capability check:", err)
	}
	source := filepath.Join(s.opts.RootDir, "album.wav")
	command := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-f", "lavfi", "-i", "sine=frequency=440:duration=3:sample_rate=44100", "-f", "lavfi", "-i", "sine=frequency=880:duration=3:sample_rate=44100", "-filter_complex", "[0:a][1:a]concat=n=2:v=0:a=1[a]", "-map", "[a]", "-c:a", "pcm_s16le", source)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture source: %v %s", err, output)
	}
	fixtureFile(t, s, "album.cue", "FILE \"album.wav\" WAVE\nTRACK 01 AUDIO\nTITLE \"First\"\nINDEX 01 00:00:00\nTRACK 02 AUDIO\nTITLE \"Second\"\nINDEX 01 00:03:00\n")
	alac := filepath.Join(s.opts.RootDir, "lossless.m4a")
	if output, err := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-i", source, "-c:a", "alac", "-metadata", "lyrics=[00:01.00]Embedded", alac).CombinedOutput(); err != nil {
		t.Fatalf("ALAC fixture: %v %s", err, output)
	}
	tracks, _ := persistScan(t, s, "actual")
	var firstID, alacID string
	for _, track := range tracks {
		if track.CueTrackNumber == 1 {
			firstID = track.ID
		}
		if track.RelativePath == "lossless.m4a" {
			alacID = track.ID
			if track.Codec != "alac" || !ResolveRepresentation(track, false).Derived {
				t.Fatal("ALAC must use MP3 representation", track.Codec)
			}
		}
	}
	if firstID == "" || alacID == "" {
		t.Fatal("audio index missing expected tracks")
	}
	selectTracks(t, s, firstID, alacID)
	media, err := s.OpenMedia(ctx, firstID, "stream", false, false)
	if err != nil {
		t.Fatal("CUE generated stream:", err)
	}
	defer media.Close()
	decoded, err := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-i", media.File.Name(), "-map", "0:a:0", "-ac", "1", "-ar", "44100", "-f", "f32le", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	count := len(decoded) / 4
	duration := float64(count) / 44100
	if duration < 2.9 || duration > 3.1 {
		t.Fatalf("CUE duration=%f", duration)
	}
	var amplitude440, amplitude880 float64
	// Compare the middle two seconds, avoiding MP3 encoder delay at boundaries.
	for index := 4410; index < count-4410; index++ {
		sample := float64(math.Float32frombits(binary.LittleEndian.Uint32(decoded[index*4 : index*4+4])))
		tsec := float64(index) / 44100
		amplitude440 += sample * math.Sin(2*math.Pi*440*tsec)
		amplitude880 += sample * math.Sin(2*math.Pi*880*tsec)
	}
	if math.Abs(amplitude440) < math.Abs(amplitude880)*10 {
		t.Fatalf("unselected neighbour leaked: 440=%f 880=%f", amplitude440, amplitude880)
	}
	alacMedia, err := s.OpenMedia(ctx, alacID, "stream", false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer alacMedia.Close()
	if alacMedia.MIME != "audio/mpeg" {
		t.Fatal("incorrect transformed MIME")
	}
	if data, err := io.ReadAll(io.LimitReader(alacMedia.File, 4)); err != nil || len(data) == 0 {
		t.Fatal("empty compatibility representation")
	}
	lyrics, err := s.GetLyrics(ctx, alacID)
	if err != nil || !lyrics.Available {
		t.Fatalf("embedded ALAC lyrics %+v %v", lyrics, err)
	}
	before, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if before.Size() < 1 {
		t.Fatal("source lost")
	}
}

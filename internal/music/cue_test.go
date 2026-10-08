package music

import (
	"encoding/binary"
	"golang.org/x/text/encoding/simplifiedchinese"
	"testing"
	"unicode/utf16"
)

func TestCUEMultipleFilesAndFrames(t *testing.T) {
	data := []byte("TITLE \"Album\"\nPERFORMER \"Artist\"\nFILE \"one.flac\" WAVE\n TRACK 01 AUDIO\n TITLE \"First\"\n INDEX 01 00:00:00\n TRACK 02 AUDIO\n INDEX 01 01:02:03\nFILE \"two.flac\" WAVE\n TRACK 03 AUDIO\n INDEX 01 00:00:10\n")
	sheet, err := parseCUE(data, "album/disc.cue")
	if err != nil {
		t.Fatal(err)
	}
	if len(sheet.Tracks) != 3 || sheet.Tracks[0].EndFrames != 4653 || sheet.Tracks[1].EndFrames != -1 || sheet.Tracks[2].File != "album/two.flac" || sheet.Tracks[2].StartFrames != 10 || sheet.Tracks[2].Artist != "Artist" {
		t.Fatalf("unexpected sheet: %+v", sheet)
	}
}
func TestCUERejectsUnsafeAndInvalidBoundaries(t *testing.T) {
	for _, body := range []string{
		"FILE \"../outside.flac\" WAVE\nTRACK 01 AUDIO\nINDEX 01 00:00:00",
		"FILE \"one.flac\" WAVE\nTRACK 01 AUDIO\nINDEX 01 00:60:00",
		"FILE \"one.flac\" WAVE\nTRACK 01 AUDIO\nINDEX 01 00:00:75",
		"FILE \"one.flac\" WAVE\nTRACK 01 AUDIO\nINDEX 01 00:01:00\nTRACK 02 AUDIO\nINDEX 01 00:00:00",
		"FILE \"one.flac\" WAVE\nTRACK 01 AUDIO",
	} {
		if _, err := parseCUE([]byte(body), "disc.cue"); err == nil {
			t.Fatalf("accepted invalid CUE: %s", body)
		}
	}
}
func TestDecodeTextEncodings(t *testing.T) {
	original := "中文歌词"
	words := utf16.Encode([]rune(original))
	little := []byte{0xff, 0xfe}
	big := []byte{0xfe, 0xff}
	for _, word := range words {
		var pair [2]byte
		binary.LittleEndian.PutUint16(pair[:], word)
		little = append(little, pair[:]...)
		binary.BigEndian.PutUint16(pair[:], word)
		big = append(big, pair[:]...)
	}
	gb, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(original))
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{[]byte(original), little, big, gb} {
		decoded, err := decodeText(data)
		if err != nil || decoded != original {
			t.Fatalf("decode: %q, %v", decoded, err)
		}
	}
	if _, err := decodeText([]byte{0xff, 0xfe, 0x00}); err == nil {
		t.Fatal("accepted odd UTF16")
	}
}

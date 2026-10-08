package music

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

const textLimit int64 = 1 << 20
const lyricsTextLimit int64 = 256 << 10

var errInvalidCUE = errors.New("invalid cue")

type cueTrack struct {
	Number                 int
	File, Title, Artist    string
	StartFrames, EndFrames int64
}
type cueSheet struct {
	Title, Artist string
	Tracks        []cueTrack
}

func decodeText(data []byte) (string, error) {
	if int64(len(data)) > textLimit {
		return "", ErrInvalid
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if len(data) >= 2 && ((data[0] == 0xff && data[1] == 0xfe) || (data[0] == 0xfe && data[1] == 0xff)) {
		little := data[0] == 0xff
		data = data[2:]
		if len(data)%2 != 0 {
			return "", ErrInvalid
		}
		words := make([]uint16, len(data)/2)
		for i := range words {
			if little {
				words[i] = binary.LittleEndian.Uint16(data[i*2:])
			} else {
				words[i] = binary.BigEndian.Uint16(data[i*2:])
			}
		}
		for i := 0; i < len(words); i++ {
			if words[i] >= 0xd800 && words[i] <= 0xdbff {
				if i+1 >= len(words) || words[i+1] < 0xdc00 || words[i+1] > 0xdfff {
					return "", ErrInvalid
				}
				i++
			} else if words[i] >= 0xdc00 && words[i] <= 0xdfff {
				return "", ErrInvalid
			}
		}
		return string(utf16.Decode(words)), nil
	}
	if utf8.Valid(data) {
		return string(data), nil
	}
	decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data)
	if err != nil || !utf8.Valid(decoded) || bytes.Contains(decoded, []byte("\ufffd")) {
		return "", ErrInvalid
	}
	return string(decoded), nil
}

func cueFields(line string) ([]string, error) {
	var fields []string
	for line = strings.TrimSpace(line); line != ""; line = strings.TrimSpace(line) {
		if line[0] == '"' {
			end := strings.IndexByte(line[1:], '"')
			if end < 0 {
				return nil, errInvalidCUE
			}
			fields = append(fields, line[1:1+end])
			line = line[end+2:]
			if line != "" && line[0] != ' ' && line[0] != '\t' {
				return nil, errInvalidCUE
			}
		} else {
			end := strings.IndexAny(line, " \t")
			if end < 0 {
				fields = append(fields, line)
				break
			}
			fields = append(fields, line[:end])
			line = line[end:]
		}
	}
	return fields, nil
}
func cueFrames(value string) (int64, error) {
	p := strings.Split(value, ":")
	if len(p) != 3 {
		return 0, errInvalidCUE
	}
	var n [3]int64
	for i, v := range p {
		if v == "" || len(v) > 8 {
			return 0, errInvalidCUE
		}
		for _, c := range v {
			if c < '0' || c > '9' {
				return 0, errInvalidCUE
			}
		}
		x, e := strconv.ParseInt(v, 10, 64)
		if e != nil {
			return 0, errInvalidCUE
		}
		n[i] = x
	}
	if n[1] >= 60 || n[2] >= 75 {
		return 0, errInvalidCUE
	}
	return (n[0]*60+n[1])*75 + n[2], nil
}
func parseCUE(data []byte, relative string) (cueSheet, error) {
	text, err := decodeText(data)
	if err != nil {
		return cueSheet{}, errInvalidCUE
	}
	sheet := cueSheet{}
	file := ""
	current := -1
	seen := map[int]bool{}
	indexed := map[int]bool{}
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 4096), int(textLimit))
	for scanner.Scan() {
		fields, e := cueFields(scanner.Text())
		if e != nil {
			return sheet, e
		}
		if len(fields) == 0 {
			continue
		}
		cmd := strings.ToUpper(fields[0])
		fields = fields[1:]
		switch cmd {
		case "FILE":
			if current >= 0 && !indexed[current] {
				return sheet, errInvalidCUE
			}
			if len(fields) != 2 || !oneOf(strings.ToUpper(fields[1]), "WAVE", "MP3", "AIFF") {
				return sheet, errInvalidCUE
			}
			name := strings.ReplaceAll(fields[0], "\\", "/")
			if _, e := secureParts(name); e != nil {
				return sheet, errInvalidCUE
			}
			file = path.Join(path.Dir(relative), name)
			if _, e := secureParts(file); e != nil {
				return sheet, errInvalidCUE
			}
			current = -1
		case "TRACK":
			if current >= 0 && !indexed[current] {
				return sheet, errInvalidCUE
			}
			if file == "" || len(fields) != 2 || strings.ToUpper(fields[1]) != "AUDIO" {
				return sheet, errInvalidCUE
			}
			number, e := strconv.Atoi(fields[0])
			if e != nil || number <= 0 || number > 99 || seen[number] {
				return sheet, errInvalidCUE
			}
			seen[number] = true
			sheet.Tracks = append(sheet.Tracks, cueTrack{Number: number, File: file, StartFrames: -1, EndFrames: -1})
			current = len(sheet.Tracks) - 1
		case "TITLE", "PERFORMER":
			if len(fields) != 1 {
				return sheet, errInvalidCUE
			}
			if current < 0 {
				if cmd == "TITLE" {
					sheet.Title = fields[0]
				} else {
					sheet.Artist = fields[0]
				}
			} else {
				if cmd == "TITLE" {
					sheet.Tracks[current].Title = fields[0]
				} else {
					sheet.Tracks[current].Artist = fields[0]
				}
			}
		case "INDEX":
			if current < 0 || len(fields) != 2 {
				return sheet, errInvalidCUE
			}
			frames, e := cueFrames(fields[1])
			if e != nil {
				return sheet, e
			}
			if fields[0] == "01" {
				if indexed[current] {
					return sheet, errInvalidCUE
				}
				sheet.Tracks[current].StartFrames = frames
				indexed[current] = true
			} else if number, parseErr := strconv.Atoi(fields[0]); parseErr != nil || number < 0 || number > 99 {
				return sheet, errInvalidCUE
			}
		case "REM", "PREGAP", "POSTGAP", "FLAGS", "CATALOG", "ISRC", "CDTEXTFILE":
			// These directives do not alter INDEX 01 boundaries.
		default:
			return sheet, fmt.Errorf("%w: directive", errInvalidCUE)
		}
	}
	if scanner.Err() != nil || len(sheet.Tracks) == 0 {
		return sheet, errInvalidCUE
	}
	last := map[string]int{}
	for i := range sheet.Tracks {
		t := &sheet.Tracks[i]
		if !indexed[i] {
			return sheet, errInvalidCUE
		}
		if prior, ok := last[t.File]; ok {
			if t.StartFrames <= sheet.Tracks[prior].StartFrames {
				return sheet, errInvalidCUE
			}
			sheet.Tracks[prior].EndFrames = t.StartFrames
		}
		last[t.File] = i
		if t.Artist == "" {
			t.Artist = sheet.Artist
		}
	}
	return sheet, nil
}

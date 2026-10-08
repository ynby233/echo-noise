package music

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var lrcTimePattern = regexp.MustCompile(`\[(\d{1,3}):(\d{2})(?:[.:](\d{1,3}))?\]`)
var lrcOffsetPattern = regexp.MustCompile(`(?i)\[offset:([+-]?\d+)\]`)
var srtTimePattern = regexp.MustCompile(`^(\d{2}):(\d{2}):(\d{2})[,.](\d{3})\s*-->`)

func parseLyricsText(text string) LyricsResponse {
	result := LyricsResponse{Lines: []LyricsLine{}}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	offset := int64(0)
	if match := lrcOffsetPattern.FindStringSubmatch(text); len(match) > 1 {
		offset, _ = strconv.ParseInt(match[1], 10, 64)
	}
	source := strings.Split(text, "\n")
	for index, line := range source {
		matches := lrcTimePattern.FindAllStringSubmatch(line, -1)
		if len(matches) > 0 {
			content := strings.TrimSpace(lrcTimePattern.ReplaceAllString(line, ""))
			if content == "" {
				continue
			}
			for _, match := range matches {
				minute, _ := strconv.ParseInt(match[1], 10, 64)
				second, _ := strconv.ParseInt(match[2], 10, 64)
				if second >= 60 {
					continue
				}
				fraction := int64(0)
				if match[3] != "" {
					fraction, _ = strconv.ParseInt((match[3] + "000")[:3], 10, 64)
				}
				ms := minute*60000 + second*1000 + fraction + offset
				if ms < 0 {
					ms = 0
				}
				result.Lines = append(result.Lines, LyricsLine{TimeMS: ms, Text: content})
			}
		} else if match := srtTimePattern.FindStringSubmatch(strings.TrimSpace(line)); len(match) > 1 {
			hour, _ := strconv.ParseInt(match[1], 10, 64)
			minute, _ := strconv.ParseInt(match[2], 10, 64)
			second, _ := strconv.ParseInt(match[3], 10, 64)
			fraction, _ := strconv.ParseInt(match[4], 10, 64)
			if minute >= 60 || second >= 60 {
				continue
			}
			body := []string{}
			for i := index + 1; i < len(source) && strings.TrimSpace(source[i]) != ""; i++ {
				body = append(body, strings.TrimSpace(source[i]))
			}
			if len(body) > 0 {
				result.Lines = append(result.Lines, LyricsLine{TimeMS: hour*3600000 + minute*60000 + second*1000 + fraction, Text: strings.Join(body, " ")})
			}
		}
	}
	sort.SliceStable(result.Lines, func(i, j int) bool { return result.Lines[i].TimeMS < result.Lines[j].TimeMS })
	if len(result.Lines) == 0 {
		result.Text = strings.TrimSpace(text)
	}
	result.Available = len(result.Lines) > 0 || result.Text != ""
	return result
}

func (s *Service) GetLyrics(ctx context.Context, trackID string) (LyricsResponse, error) {
	track, err := s.checkTrack(ctx, trackID, false)
	if err != nil {
		return LyricsResponse{}, err
	}
	empty := LyricsResponse{Lines: []LyricsLine{}}
	var text string
	switch track.LyricsKind {
	case "embedded":
		text, err = s.embeddedLyrics(ctx, track)
	case "sidecar":
		var data []byte
		data, err = s.secureRead(ctx, track.LyricsRelativePath, 256<<10)
		if err == nil {
			text, err = decodeText(data)
		}
	default:
		return empty, nil
	}
	if err != nil {
		return empty, nil
	}
	if len(text) > 256<<10 {
		return empty, nil
	}
	result := parseLyricsText(text)
	if track.CueTrackNumber > 0 && len(result.Lines) > 0 {
		start, end := track.StartFrames*1000/75, track.EndFrames*1000/75
		clipped := make([]LyricsLine, 0, len(result.Lines))
		for _, line := range result.Lines {
			if line.TimeMS >= start && line.TimeMS < end {
				line.TimeMS -= start
				clipped = append(clipped, line)
			}
		}
		result.Lines = clipped
		result.Available = len(clipped) > 0
	}
	if err = s.mediaStillEligible(ctx, track, false); err != nil {
		return LyricsResponse{}, err
	}
	return result, nil
}

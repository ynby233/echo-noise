package music

import (
	"context"
	"errors"
	"regexp"

	"github.com/rcy1314/echo-noise/internal/models"
)

var ErrNoteTrackInvalid = errors.New("笔记音乐标记已失效，请重新选择歌曲")
var noteMusicLink = regexp.MustCompile(`\]\(/music/notes/([^\)\r\n]*)\)`)
var noteMusicID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type noteTrackAccessKey struct{}

// WithNoteTrackAccess is only for callers that have already authorized a visible
// note containing this exact track. It bypasses playlist membership, never the
// music switch, source-file validation, or authorization of another track.
func WithNoteTrackAccess(ctx context.Context, trackID string) context.Context {
	return context.WithValue(ctx, noteTrackAccessKey{}, trackID)
}

func carryNoteTrackAccess(target, source context.Context) context.Context {
	if trackID, ok := source.Value(noteTrackAccessKey{}).(string); ok {
		return WithNoteTrackAccess(target, trackID)
	}
	return target
}

// NoteTrackIDs recognizes the canonical Markdown link destination. Counts are
// retained so adding another occurrence also requires music.manage.
func NoteTrackIDs(content string) ([]string, error) {
	ids := []string{}
	for _, match := range noteMusicLink.FindAllStringSubmatch(content, -1) {
		if !noteMusicID.MatchString(match[1]) {
			return nil, ErrNoteTrackInvalid
		}
		ids = append(ids, match[1])
	}
	return ids, nil
}

func publicTrack(track models.MusicTrack) PublicTrack {
	rep := ResolveRepresentation(track, false)
	cover := ""
	if track.CoverKind != "none" && track.CoverKind != "" {
		cover = "/api/music/cover/" + track.ID
	}
	return PublicTrack{TrackID: track.ID, Title: track.Title, Artist: track.Artist, Album: track.Album, DurationMS: track.DurationMS, Format: track.Format, MIMEType: rep.MIME, CoverURL: cover, LyricsAvailable: track.LyricsKind != "none" && track.LyricsKind != "", LyricsURL: "/api/music/lyrics/" + track.ID, StreamURL: "/api/music/stream/" + track.ID, FallbackStreamURL: "/api/music/stream/" + track.ID + "?compat=1", MediaVersion: rep.Key}
}

// ResolveNoteTrack returns a safe playback projection without editing the global
// playlist. The HTTP caller must first verify note visibility and its reference.
func (s *Service) ResolveNoteTrack(ctx context.Context, trackID string) (PublicTrack, error) {
	track, err := s.checkTrack(WithNoteTrackAccess(ctx, trackID), trackID, false)
	if err != nil {
		return PublicTrack{}, ErrNoteTrackInvalid
	}
	return publicTrack(track), nil
}

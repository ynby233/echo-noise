// Package music implements the independent, read-only global music library.
package music

import (
	"context"
	"errors"
	"io"
	"os"
	"time"
)

var (
	ErrInvalid       = errors.New("音乐请求参数无效")
	ErrForbidden     = errors.New("没有音乐管理权限")
	ErrConflict      = errors.New("音乐配置已被其他管理员更新，请重新加载后保存")
	ErrUnprocessable = errors.New("所选歌曲不可用")
	ErrNotFound      = errors.New("音乐资源不可用")
	ErrUnavailable   = errors.New("音频处理暂时不可用")
	ErrBusy          = errors.New("音频正在准备，请稍后重试")
)

// ToolRunner must respect ctx and the output limits, and must not reopen input.File by path.
type ToolRunner interface {
	Run(ctx context.Context, executable string, args []string, input ToolInput, stdout io.Writer) error
}

type ToolInput struct {
	File  *os.File
	Stdin io.Reader
}

type Options struct {
	RootDir       string
	CacheDir      string
	FFmpegPath    string
	FFprobePath   string
	ExcludedRoots []string
	Tools         ToolRunner
	Now           func() time.Time
	// Test hooks run before verified fd access; they cannot override root confinement.
	BeforeOpen func(string)
}

type SaveConfigRequest struct {
	Version             uint64                 `json:"version"`
	FrontendSettings    map[string]interface{} `json:"frontendSettings"`
	ScanIntervalMinutes int                    `json:"scanIntervalMinutes"`
	TrackIDs            []string               `json:"trackIDs"`
}

type LibraryQuery struct {
	Page         int
	PageSize     int
	Q            string
	Format       string
	Lyrics       string
	Availability string
	Selected     string
}

type AdminTrack struct {
	TrackID         string `json:"trackID"`
	Title           string `json:"title"`
	Artist          string `json:"artist"`
	Album           string `json:"album"`
	DurationMS      int64  `json:"durationMS"`
	Format          string `json:"format"`
	CoverURL        string `json:"coverURL"`
	LyricsAvailable bool   `json:"lyricsAvailable"`
	Available       bool   `json:"available"`
	FailureCode     string `json:"failureCode"`
	CueTrackNumber  int    `json:"cueTrackNumber"`
	Selected        bool   `json:"selected"`
}

type LibraryPage struct {
	Items    []AdminTrack `json:"items"`
	Total    int64        `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"pageSize"`
}

type PlaylistResponse struct {
	Version uint64       `json:"version"`
	Items   []AdminTrack `json:"items"`
}

type LibraryCounts struct {
	Total       int64 `json:"total"`
	Available   int64 `json:"available"`
	Unavailable int64 `json:"unavailable"`
}

type ScanStatus struct {
	RunID            string     `json:"runID"`
	State            string     `json:"state"`
	StartedAt        *time.Time `json:"startedAt"`
	FinishedAt       *time.Time `json:"finishedAt"`
	LastSuccessAt    *time.Time `json:"lastSuccessAt"`
	LastSuccessRunID string     `json:"lastSuccessRunID"`
	NextAutoScanAt   time.Time  `json:"nextAutoScanAt"`
	Processed        int        `json:"processed"`
	Discovered       int        `json:"discovered"`
	Available        int        `json:"available"`
	Unavailable      int        `json:"unavailable"`
	MetadataFailed   int        `json:"metadataFailed"`
	InvalidCUE       int        `json:"invalidCUE"`
	ErrorCode        string     `json:"errorCode"`
	ErrorMessage     string     `json:"errorMessage"`
}

type AdminConfig struct {
	Version             uint64                 `json:"version"`
	FrontendSettings    map[string]interface{} `json:"frontendSettings"`
	ScanIntervalMinutes int                    `json:"scanIntervalMinutes"`
	Playlist            []AdminTrack           `json:"playlist"`
	Scan                ScanStatus             `json:"scan"`
	RootReadable        bool                   `json:"rootReadable"`
	ToolsReady          bool                   `json:"toolsReady"`
	Counts              LibraryCounts          `json:"counts"`
}

type PublicMusic struct {
	Source           string                 `json:"source"`
	Revision         string                 `json:"revision"`
	FrontendSettings map[string]interface{} `json:"frontendSettings"`
	Tracks           []PublicTrack          `json:"tracks"`
}

type PublicTrack struct {
	TrackID           string `json:"trackID"`
	Title             string `json:"title"`
	Artist            string `json:"artist"`
	Album             string `json:"album"`
	DurationMS        int64  `json:"durationMS"`
	Format            string `json:"format"`
	MIMEType          string `json:"mimeType"`
	CoverURL          string `json:"coverURL"`
	LyricsAvailable   bool   `json:"lyricsAvailable"`
	LyricsURL         string `json:"lyricsURL"`
	StreamURL         string `json:"streamURL"`
	FallbackStreamURL string `json:"fallbackStreamURL"`
	MediaVersion      string `json:"mediaVersion"`
}

type LyricsLine struct {
	TimeMS int64  `json:"timeMS"`
	Text   string `json:"text"`
}

type LyricsResponse struct {
	Available bool         `json:"available"`
	Lines     []LyricsLine `json:"lines"`
	Text      string       `json:"text"`
}

// Representation is the actual byte representation, not the source's display format.
type Representation struct {
	MIME    string
	Derived bool
	Key     string
}

// MediaFile is an already-authorized, seekable representation for http.ServeContent.
// Call Close after serving so capacity eviction cannot remove an active representation.
type MediaFile struct {
	File    *os.File
	Info    os.FileInfo
	MIME    string
	ETag    string
	Name    string
	release func()
}

func (m *MediaFile) Close() error {
	if m == nil {
		return nil
	}
	var err error
	if m.File != nil {
		err = m.File.Close()
		m.File = nil
	}
	if m.release != nil {
		m.release()
		m.release = nil
	}
	return err
}

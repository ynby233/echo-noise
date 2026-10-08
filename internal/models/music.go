package models

import "time"

// MusicConfig is the singleton configuration revision shared by every music writer.
type MusicConfig struct {
	ID                  uint   `gorm:"primaryKey"`
	Source              string `gorm:"type:varchar(16);not null;default:netease"`
	ScanIntervalMinutes int    `gorm:"not null;default:60"`
	Version             uint64 `gorm:"not null;default:1"`
	LibraryVersion      uint64 `gorm:"not null;default:0"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// MusicTrack never leaves the service directly: filesystem fields are private projections.
type MusicTrack struct {
	ID                 string    `gorm:"type:varchar(36);primaryKey" json:"-"`
	CatalogKey         string    `gorm:"type:varchar(64);not null;uniqueIndex" json:"-"`
	RelativePath       string    `gorm:"type:text;not null" json:"-"`
	CueRelativePath    string    `gorm:"type:text" json:"-"`
	CueTrackNumber     int       `gorm:"not null;default:0" json:"-"`
	StartFrames        int64     `gorm:"not null;default:0" json:"-"`
	EndFrames          int64     `gorm:"not null;default:0" json:"-"`
	Title              string    `gorm:"type:text" json:"-"`
	Artist             string    `gorm:"type:text" json:"-"`
	Album              string    `gorm:"type:text" json:"-"`
	DurationMS         int64     `gorm:"not null;default:0" json:"-"`
	Format             string    `gorm:"type:varchar(16);index" json:"-"`
	Codec              string    `gorm:"type:varchar(64)" json:"-"`
	MIMEType           string    `gorm:"type:varchar(64)" json:"-"`
	FileSize           int64     `json:"-"`
	ModifiedNS         int64     `json:"-"`
	Fingerprint        string    `gorm:"type:varchar(64)" json:"-"`
	CoverKind          string    `gorm:"type:varchar(16);not null;default:none" json:"-"`
	CoverRelativePath  string    `gorm:"type:text" json:"-"`
	CoverStreamIndex   int       `json:"-"`
	LyricsKind         string    `gorm:"type:varchar(16);not null;default:none" json:"-"`
	LyricsRelativePath string    `gorm:"type:text" json:"-"`
	LyricsTag          string    `gorm:"type:varchar(191)" json:"-"`
	Available          bool      `gorm:"not null;default:false;index" json:"-"`
	FailureCode        string    `gorm:"type:varchar(32)" json:"-"`
	LastSeenRunID      string    `gorm:"type:varchar(36);index" json:"-"`
	CreatedAt          time.Time `json:"-"`
	UpdatedAt          time.Time `json:"-"`
}

// MusicPlaylistItem is the single ordered, global playlist; unavailable items remain editable.
type MusicPlaylistItem struct {
	ID        uint   `gorm:"primaryKey"`
	TrackID   string `gorm:"type:varchar(36);not null;uniqueIndex"`
	Position  int    `gorm:"not null;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type MusicScanState struct {
	ID               uint   `gorm:"primaryKey"`
	RunID            string `gorm:"type:varchar(36)"`
	State            string `gorm:"type:varchar(16);not null;default:idle"`
	StartedAt        *time.Time
	FinishedAt       *time.Time
	LastSuccessAt    *time.Time
	LastSuccessRunID string `gorm:"type:varchar(36)"`
	NextAutoScanAt   time.Time
	Discovered       int
	Available        int
	Unavailable      int
	MetadataFailed   int
	InvalidCUE       int
	ErrorCode        string `gorm:"type:varchar(32)"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

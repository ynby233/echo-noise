package music

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	db           *gorm.DB
	opts         Options
	mu           sync.Mutex
	status       ScanStatus
	toolsReady   bool
	toolError    string
	lifecycleCtx context.Context
	cancel       context.CancelFunc
	mediaSlots   chan struct{}
	cache        *mediaCache
	wg           sync.WaitGroup
	started      bool
	closed       bool
	running      bool
	wake         chan struct{}
}

func NewService(db *gorm.DB, opts Options) *Service {
	if opts.RootDir == "" {
		opts.RootDir = "/app/music"
	}
	if opts.CacheDir == "" {
		opts.CacheDir = "data/music-cache"
	}
	if opts.FFmpegPath == "" {
		opts.FFmpegPath = "ffmpeg"
	}
	if opts.FFprobePath == "" {
		opts.FFprobePath = "ffprobe"
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Tools == nil {
		opts.Tools = commandRunner{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{db: db, opts: opts, lifecycleCtx: ctx, cancel: cancel, mediaSlots: make(chan struct{}, 2), wake: make(chan struct{}, 1), status: ScanStatus{State: "idle"}}
	s.cache = newMediaCache(opts)
	return s
}

func statusFromModel(state models.MusicScanState) ScanStatus {
	return ScanStatus{RunID: state.RunID, State: state.State, StartedAt: state.StartedAt, FinishedAt: state.FinishedAt, LastSuccessAt: state.LastSuccessAt, LastSuccessRunID: state.LastSuccessRunID, NextAutoScanAt: state.NextAutoScanAt, Discovered: state.Discovered, Processed: state.Discovered, Available: state.Available, Unavailable: state.Unavailable, MetadataFailed: state.MetadataFailed, InvalidCUE: state.InvalidCUE, ErrorCode: state.ErrorCode, ErrorMessage: scanErrorMessage(state.ErrorCode)}
}

func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started || s.closed {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	s.cache.start(s.lifecycleCtx)
	go func() {
		defer s.wg.Done()
		defer s.cancel()
		if s.db == nil {
			return
		}
		if err := s.recoverState(ctx); err != nil {
			s.progress(func(st *ScanStatus) {
				st.State = "failed"
				st.ErrorCode = "io_error"
				st.ErrorMessage = scanErrorMessage("io_error")
			})
			return
		}
		// Tool availability is process state, independent of the persisted scan schedule.
		// Probe after every restart even when the last successful index is still fresh.
		_ = s.checkTools(ctx)
		s.autoScan()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.lifecycleCtx.Done():
				return
			case <-ticker.C:
				s.autoScan()
			case <-s.wake:
				s.refreshPersistedStatus()
			}
		}
	}()
}

func (s *Service) recoverState(ctx context.Context) error {
	var recovered ScanStatus
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := EnsureDefaults(tx); err != nil {
			return err
		}
		var state models.MusicScanState
		if err := tx.First(&state, 1).Error; err != nil {
			return err
		}
		if state.State == "running" {
			now := s.opts.Now()
			var config models.MusicConfig
			if err := tx.First(&config, 1).Error; err != nil {
				return err
			}
			state.State = "failed"
			state.ErrorCode = "interrupted"
			state.FinishedAt = &now
			state.NextAutoScanAt = now.Add(time.Duration(config.ScanIntervalMinutes) * time.Minute)
			if err := tx.Save(&state).Error; err != nil {
				return err
			}
		}
		recovered = statusFromModel(state)
		return nil
	})
	if err == nil {
		s.progress(func(st *ScanStatus) { *st = recovered })
	}
	return err
}

func (s *Service) autoScan() {
	if s.lifecycleCtx.Err() != nil {
		return
	}
	var state models.MusicScanState
	if s.db.WithContext(s.lifecycleCtx).First(&state, 1).Error != nil {
		return
	}
	now := s.opts.Now()
	if state.NextAutoScanAt.IsZero() || !state.NextAutoScanAt.After(now) {
		_, _, _ = s.requestScan(s.lifecycleCtx, 0, true)
	}
}

func (s *Service) Wait(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	cacheDone := make(chan error, 1)
	go func() { cacheDone <- s.cache.wait(context.Background()) }()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-cacheDone:
		return err
	}
}

func (s *Service) progress(fn func(*ScanStatus)) { s.mu.Lock(); defer s.mu.Unlock(); fn(&s.status) }
func (s *Service) GetScanStatus() ScanStatus     { s.mu.Lock(); defer s.mu.Unlock(); return s.status }
func (s *Service) notifySchedule() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
func (s *Service) refreshPersistedStatus() {
	var state models.MusicScanState
	if s.db.WithContext(s.lifecycleCtx).First(&state, 1).Error == nil {
		s.mu.Lock()
		if !s.running {
			s.status = statusFromModel(state)
		} else {
			s.status.NextAutoScanAt = state.NextAutoScanAt
		}
		s.mu.Unlock()
	}
}

func (s *Service) RequestScan(ctx context.Context, actorID uint) (ScanStatus, bool, error) {
	return s.requestScan(ctx, actorID, false)
}
func (s *Service) requestScan(ctx context.Context, actorID uint, system bool) (ScanStatus, bool, error) {
	if s.db == nil {
		return ScanStatus{}, false, ErrUnavailable
	}
	if !system {
		if err := authorizeMusic(s.db.WithContext(ctx), actorID); err != nil {
			return ScanStatus{}, false, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.lifecycleCtx.Err() != nil {
		return s.status, false, ErrUnavailable
	}
	if s.running {
		return s.status, false, nil
	}
	now := s.opts.Now()
	runID := uuid.NewString()
	var state models.MusicScanState
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := EnsureDefaults(tx); err != nil {
			return err
		}
		if !system {
			if err := authorizeMusic(tx, actorID); err != nil {
				return err
			}
		}
		if err := tx.First(&state, 1).Error; err != nil {
			return err
		}
		state.RunID = runID
		state.State = "running"
		state.StartedAt = &now
		state.FinishedAt = nil
		state.ErrorCode = ""
		state.Discovered = 0
		state.Available = 0
		state.Unavailable = 0
		state.MetadataFailed = 0
		state.InvalidCUE = 0
		if err := tx.Save(&state).Error; err != nil {
			return err
		}
		return writeMusicAudit(tx, actorID, "refresh", map[string]interface{}{"runID": runID})
	})
	if err != nil {
		return s.status, false, err
	}
	s.running = true
	s.status = statusFromModel(state)
	s.status.Processed = 0
	s.wg.Add(1)
	go s.runScan(runID)
	return s.status, true, nil
}

func (s *Service) runScan(runID string) {
	defer s.wg.Done()
	ctx, cancel := context.WithTimeout(s.lifecycleCtx, 30*time.Minute)
	defer cancel()
	var old []models.MusicTrack
	err := s.db.WithContext(ctx).Find(&old).Error
	var tracks []models.MusicTrack
	var outcome ScanStatus
	if err == nil {
		tracks, outcome, err = s.collectScan(ctx, runID, old)
	}
	code := ""
	if err != nil {
		code = scanCode(err)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code = "scan_timeout"
		} else if errors.Is(ctx.Err(), context.Canceled) {
			code = "interrupted"
		}
		if code == "" {
			code = "io_error"
		}
	}
	// Shutdown keeps the DB alive until Wait; result persistence has a bounded,
	// independent context because the scan's cancellation must still be recorded.
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finishCancel()
	var completed ScanStatus
	err = s.db.WithContext(finishCtx).Transaction(func(tx *gorm.DB) error {
		var state models.MusicScanState
		var config models.MusicConfig
		if err := tx.First(&state, 1).Error; err != nil {
			return err
		}
		if err := tx.First(&config, 1).Error; err != nil {
			return err
		}
		now := s.opts.Now()
		state.FinishedAt = &now
		state.NextAutoScanAt = now.Add(time.Duration(config.ScanIntervalMinutes) * time.Minute)
		state.ErrorCode = code
		if code == "" {
			if len(tracks) > 0 {
				if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "catalog_key"}}, DoUpdates: clause.AssignmentColumns([]string{"relative_path", "cue_relative_path", "cue_track_number", "start_frames", "end_frames", "title", "artist", "album", "duration_ms", "format", "codec", "mime_type", "file_size", "modified_ns", "fingerprint", "cover_kind", "cover_relative_path", "cover_stream_index", "lyrics_kind", "lyrics_relative_path", "lyrics_tag", "available", "failure_code", "last_seen_run_id", "updated_at"})}).CreateInBatches(tracks, 100).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&models.MusicTrack{}).Where("last_seen_run_id <> ? OR last_seen_run_id IS NULL", runID).Updates(map[string]interface{}{"available": false, "failure_code": "missing_source"}).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.MusicConfig{}).Where("id = ?", 1).Update("library_version", gorm.Expr("library_version + 1")).Error; err != nil {
				return err
			}
			state.State = "succeeded"
			state.LastSuccessAt = &now
			state.LastSuccessRunID = runID
			state.Discovered = outcome.Discovered
			state.Available = outcome.Available
			state.Unavailable = outcome.Unavailable
			state.MetadataFailed = outcome.MetadataFailed
			state.InvalidCUE = outcome.InvalidCUE
		} else {
			state.State = "failed"
		}
		if err := tx.Save(&state).Error; err != nil {
			return err
		}
		completed = statusFromModel(state)
		return nil
	})
	s.mu.Lock()
	s.running = false
	if err == nil {
		s.status = completed
	}
	if err != nil {
		s.status.State = "failed"
		s.status.ErrorCode = "io_error"
		s.status.ErrorMessage = scanErrorMessage("io_error")
	}
	s.mu.Unlock()
}

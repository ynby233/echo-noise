package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

func registerCommentSummaryRoutes(r *gin.Engine) {
	r.Use(func(c *gin.Context) {
		if id, ok := commentUint(c.GetHeader("X-Test-User-ID")); ok {
			c.Set("user_id", id)
		}
		c.Next()
	})
	r.POST("/messages/comments/counts", GetCommentCounts)
	r.GET("/messages/:id/comments", GetComments)
}

func requestCommentSummaries(t *testing.T, r http.Handler, actorID uint, ids []uint) []services.CommentSummary {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"ids": ids})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/messages/comments/counts", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if actorID != 0 {
		req.Header.Set("X-Test-User-ID", strconvFormatUint(actorID))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var response struct {
		Code int                       `json:"code"`
		Data []services.CommentSummary `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != http.StatusOK || response.Code != 1 || response.Data == nil {
		t.Fatalf("summary request failed: %d %s, error %v", w.Code, w.Body.String(), err)
	}
	if len(response.Data) > 0 && (!bytes.Contains(w.Body.Bytes(), []byte(`"hasHiddenComments":false`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"hasVisibleComments":`))) {
		t.Fatalf("summary flags must be explicit: %s", w.Body.String())
	}
	for _, row := range response.Data {
		if row.HasHiddenComments {
			t.Fatalf("must not disclose unreadable comment existence: %+v", row)
		}
	}
	return response.Data
}

func TestCommentSummariesVisibilityAndHiddenHint(t *testing.T) {
	db, r, owner, message := setupCommentAccountTest(t)
	author := models.User{Username: "summary-author"}
	outsider := models.User{Username: "summary-outsider"}
	for _, user := range []*models.User{&author, &outsider} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	parent := createTestComment(t, db, message.ID, &author, "public", "public", nil)
	createTestComment(t, db, message.ID, &author, "users", "users", nil)
	createTestComment(t, db, message.ID, &author, "private", "private", nil)
	createTestComment(t, db, message.ID, &author, "reply", "public", &parent.ID)
	registerCommentSummaryRoutes(r)
	for _, tc := range []struct {
		name  string
		actor uint
		count int64
	}{
		{"anonymous", 0, 1},
		{"logged-in", outsider.ID, 2},
		{"comment-author", author.ID, 3},
		{"note-owner", owner.ID, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := requestCommentSummaries(t, r, tc.actor, []uint{message.ID})
			if len(rows) != 1 || rows[0].Count != tc.count || !rows[0].HasVisibleComments {
				t.Fatalf("unexpected summary: %+v", rows)
			}
		})
	}
}

func TestCommentSummariesLifecycleAndInvisibleAncestors(t *testing.T) {
	for _, scenario := range []string{
		"hidden-parent-chain", "private-tombstone", "deleted-comment", "empty-tombstone",
		"visible-reply-below-tombstone", "deleted-message", "message-tombstone", "private-message-tombstone",
	} {
		t.Run(scenario, func(t *testing.T) {
			db, r, owner, message := setupCommentAccountTest(t)
			parent := createTestComment(t, db, message.ID, &owner, "parent", "public", nil)
			now := time.Now().UTC()
			wantVisible, wantOmitted := false, false
			switch scenario {
			case "hidden-parent-chain":
				if err := db.Model(&parent).Update("visibility", "private").Error; err != nil {
					t.Fatal(err)
				}
				middle := createTestComment(t, db, message.ID, &owner, "middle", "public", &parent.ID)
				if err := db.Model(&middle).Updates(map[string]any{"is_tombstone": true, "deleted_at": now, "tombstone_visibility": "public"}).Error; err != nil {
					t.Fatal(err)
				}
				createTestComment(t, db, message.ID, &owner, "reply", "public", &middle.ID)
			case "private-tombstone", "empty-tombstone", "visible-reply-below-tombstone":
				visibility := "public"
				if scenario == "private-tombstone" {
					visibility = "private"
				}
				if err := db.Model(&parent).Updates(map[string]any{"is_tombstone": true, "deleted_at": now, "visibility": "public", "tombstone_visibility": visibility}).Error; err != nil {
					t.Fatal(err)
				}
				if scenario != "empty-tombstone" {
					createTestComment(t, db, message.ID, &owner, "reply", "public", &parent.ID)
				}
				wantVisible = scenario == "visible-reply-below-tombstone"
			case "deleted-comment":
				if err := db.Model(&parent).Update("deleted_at", now).Error; err != nil {
					t.Fatal(err)
				}
			case "deleted-message", "message-tombstone", "private-message-tombstone":
				visibility := "public"
				if scenario == "private-message-tombstone" {
					visibility = "private"
				}
				if err := db.Model(&message).Updates(map[string]any{"deleted_at": now, "is_tombstone": scenario != "deleted-message", "tombstone_visibility": visibility}).Error; err != nil {
					t.Fatal(err)
				}
				wantOmitted = scenario != "message-tombstone"
				wantVisible = !wantOmitted
			}
			registerCommentSummaryRoutes(r)
			rows := requestCommentSummaries(t, r, 0, []uint{message.ID})
			if wantOmitted {
				if len(rows) != 0 {
					t.Fatalf("unreadable note must be omitted: %+v", rows)
				}
				return
			}
			wantCount := int64(0)
			if scenario == "message-tombstone" {
				wantCount = 1
			}
			if len(rows) != 1 || rows[0].Count != wantCount || rows[0].HasVisibleComments != wantVisible {
				t.Fatalf("unexpected summary: %+v", rows)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/messages/"+strconvFormatUint(message.ID)+"/comments", nil))
			list := decodeCommentListResponse(t, w)
			if rows[0].HasVisibleComments != (len(list) > 0) {
				t.Fatalf("expansion must match GetComments: summary=%+v, list=%+v", rows[0], list)
			}
		})
	}
}

func TestCommentSummariesDelegatedAdminProtection(t *testing.T) {
	db, r, primary, message := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
		t.Fatal(err)
	}
	primary.IsAdmin = true
	if err := db.Save(&primary).Error; err != nil {
		t.Fatal(err)
	}
	delegated := models.User{Username: "summary-delegated", IsAdmin: true}
	ordinary := models.User{Username: "summary-ordinary"}
	for _, user := range []*models.User{&delegated, &ordinary} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	createTestComment(t, db, message.ID, &ordinary, "ordinary private", "private", nil)
	protected := createTestComment(t, db, message.ID, &primary, "protected", "private", nil)
	createTestComment(t, db, message.ID, &ordinary, "protected descendant", "public", &protected.ID)
	privateNotes := []models.Message{
		{UserID: primary.ID, Visibility: "private", Content: "protected note"},
		{UserID: ordinary.ID, Visibility: "private", Content: "ordinary note"},
	}
	if err := db.Create(&privateNotes).Error; err != nil {
		t.Fatal(err)
	}
	registerCommentSummaryRoutes(r)
	ids := []uint{message.ID, privateNotes[0].ID, privateNotes[1].ID}
	for _, tc := range []struct {
		name   string
		grants []authorization.Capability
		rows   int
		count  int64
	}{
		{"no-grants", nil, 1, 0},
		{"interactions-only", []authorization.Capability{authorization.CapabilityCommentsView, authorization.CapabilityCommentsViewHidden}, 1, 1},
		{"notes-and-interactions", []authorization.Capability{authorization.CapabilityCommentsView, authorization.CapabilityCommentsViewHidden, authorization.CapabilityNotesView, authorization.CapabilityNotesViewHidden}, 2, 1},
		{"revoked", nil, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, tc.grants); err != nil {
				t.Fatal(err)
			}
			rows := requestCommentSummaries(t, r, delegated.ID, ids)
			if len(rows) != tc.rows || rows[0].Count != tc.count || rows[0].HasVisibleComments != (tc.count > 0) {
				t.Fatalf("unexpected delegated summary: %+v", rows)
			}
			for _, row := range rows {
				if row.ID == privateNotes[0].ID {
					t.Fatalf("protected note disclosed: %+v", row)
				}
			}
		})
	}
}

func TestCommentSummariesBatchLimitDeduplicationAndZero(t *testing.T) {
	db, r, owner, message := setupCommentAccountTest(t)
	private := models.Message{UserID: owner.ID, Visibility: "private", Content: "private"}
	if err := db.Create(&private).Error; err != nil {
		t.Fatal(err)
	}
	registerCommentSummaryRoutes(r)
	rows := requestCommentSummaries(t, r, 0, []uint{0, message.ID, private.ID, message.ID, 999999})
	if len(rows) != 1 || rows[0].ID != message.ID || rows[0].Count != 0 || rows[0].HasVisibleComments {
		t.Fatalf("must deduplicate, omit unreadable/missing notes, and keep readable zero: %+v", rows)
	}
	ids := make([]uint, 100)
	for i := range ids {
		ids[i] = message.ID
	}
	if got := requestCommentSummaries(t, r, 0, ids); len(got) != 1 {
		t.Fatalf("100 IDs must be accepted and deduplicated: %+v", got)
	}
	tooMany, _ := json.Marshal(map[string]any{"ids": append(ids, message.ID)})
	for _, body := range []string{`{}`, `{"ids":[]}`, `{"ids":[-1]}`, `{"ids":["1"]}`, string(tooMany)} {
		req := httptest.NewRequest(http.MethodPost, "/messages/comments/counts", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid or oversized request must return 400, got %d: %s", w.Code, w.Body.String())
		}
	}
}

package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

type messagePaginationQuery struct {
	sql  string
	rows int64
}

func recordMessagePaginationQueries(t *testing.T, db *gorm.DB) *[]messagePaginationQuery {
	t.Helper()
	queries := &[]messagePaginationQuery{}
	const callback = "test:message-pagination-query"
	if err := db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "messages" {
			*queries = append(*queries, messagePaginationQuery{sql: tx.Statement.SQL.String(), rows: tx.RowsAffected})
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Callback().Query().Remove(callback) })
	return queries
}

func TestMessagePaginationScansBoundedExactTagBatchesAndHydratesOnlyPage(t *testing.T) {
	db := setupUserServiceTestDB(t)
	owner := mustCreateUser(t, models.User{ID: 42, Username: "pagination-owner"})
	base := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	pinTime := base.Add(time.Hour)
	contents := []string{"#标签，匹配", "#标签 匹配", "#标签后缀", "#标签?id=123", "#标签/path", "#标签&x=y"}
	messages := make([]models.Message, messageTagScanBatchSize*3+21)
	for i := range messages {
		messages[i] = models.Message{
			Content: contents[i%len(contents)], UserID: owner.ID, Username: owner.Username,
			Visibility: MessageVisibilityPublic, CreatedAt: base.Add(time.Duration(i%3) * time.Minute),
			Pinned: i%3 == 0, PersonalPinned: i%3 == 1,
			ImageURL: fmt.Sprintf("image-%d", i), LikeCount: i,
		}
		if i%2 == 0 {
			messages[i].PinnedAt = &pinTime
		}
		if i%4 == 0 {
			messages[i].PersonalPinnedAt = &pinTime
		}
	}
	if err := db.CreateInBatches(&messages, 50).Error; err != nil {
		t.Fatal(err)
	}
	// Both flags and created_at permit NULL in historical/imported rows.
	if err := db.Model(&models.Message{}).Where("id >= ?", messages[messageTagScanBatchSize*2].ID).
		Updates(map[string]interface{}{"pinned": nil, "personal_pinned": nil}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.Message{}).Where("id % 5 = 0").Update("created_at", nil).Error; err != nil {
		t.Fatal(err)
	}
	queries := recordMessagePaginationQueries(t, db)
	tag := " 标签 "
	for _, pinScope := range []string{MessagePinScopeLatest, MessagePinScopePersonal} {
		t.Run(pinScope, func(t *testing.T) {
			var ordered []models.Message
			if err := db.Order(messagePinOrder(pinScope)).Find(&ordered).Error; err != nil {
				t.Fatal(err)
			}
			matched := make([]models.Message, 0)
			for _, message := range ordered {
				if messageMatchesTag(message.Content, tag) {
					matched = append(matched, message)
				}
			}
			const pageSize = 37
			lastPage := (len(matched) + pageSize - 1) / pageSize
			for _, page := range []int{1, 3, lastPage, lastPage + 1, int(^uint(0) >> 1)} {
				*queries = nil
				result, err := GetMessagesByPage(page, pageSize, &owner.ID, &owner.ID, nil, nil, nil, &tag, pinScope, nil)
				if err != nil {
					t.Fatalf("page %d: %v", page, err)
				}
				if result.Total != int64(len(matched)) {
					t.Fatalf("page %d total=%d want %d", page, result.Total, len(matched))
				}
				var want []models.Message
				for index, message := range matched {
					if index/pageSize+1 == page {
						want = append(want, message)
					}
				}
				if len(result.Items) != len(want) {
					t.Fatalf("page %d length=%d want %d", page, len(result.Items), len(want))
				}
				for index, message := range result.Items {
					if message.ID != want[index].ID || message.ImageURL != want[index].ImageURL || message.LikeCount != want[index].LikeCount || !message.CanInteract {
						t.Fatalf("page %d item %d not fully hydrated: %#v want %#v", page, index, message, want[index])
					}
				}
				fullReads, scans := 0, 0
				for _, query := range *queries {
					if strings.Contains(query.sql, "SELECT *") {
						fullReads++
						if query.rows > pageSize || !strings.Contains(query.sql, "id IN") {
							t.Fatalf("unbounded hydrate: %s (%d rows)", query.sql, query.rows)
						}
					} else if strings.Contains(query.sql, ", content") {
						scans++
						if query.rows > messageTagScanBatchSize || !strings.Contains(query.sql, "LIMIT 256") || strings.Contains(query.sql, "OFFSET") {
							t.Fatalf("unbounded tag scan: %s (%d rows)", query.sql, query.rows)
						}
					}
				}
				wantFullReads := 0
				if len(want) > 0 {
					wantFullReads = 1
				}
				if fullReads != wantFullReads || scans < 4 {
					t.Fatalf("full reads=%d want %d; scans=%d want >=4", fullReads, wantFullReads, scans)
				}
			}
			*queries = nil
			target := matched[len(matched)-1]
			location, err := LocateMessagePage(target.ID, pageSize, &owner.ID, &owner.ID, nil, nil, nil, &tag, pinScope, nil)
			if err != nil || location.Page != lastPage || location.Total != int64(len(matched)) {
				t.Fatalf("tag locate=%#v err=%v", location, err)
			}
			for _, query := range *queries {
				if strings.Contains(query.sql, "SELECT *") || query.rows > messageTagScanBatchSize {
					t.Fatalf("tag locate read full/unbounded rows: %#v", query)
				}
			}
			for _, index := range []int{0, 1, 255, 256, 511, 512, len(ordered) - 1} {
				*queries = nil
				location, err := LocateMessagePage(ordered[index].ID, pageSize, &owner.ID, &owner.ID, nil, nil, nil, nil, pinScope, nil)
				if err != nil || location.Page != index/pageSize+1 || location.Total != int64(len(ordered)) {
					t.Fatalf("ordinary locate index=%d: %#v err=%v", index, location, err)
				}
				for _, query := range *queries {
					if strings.Contains(query.sql, "SELECT *") || strings.Contains(query.sql, "content") || query.rows > 1 {
						t.Fatalf("ordinary locate hydrated candidates: %#v", query)
					}
				}
			}
		})
	}
}

func TestMessageLocateAndTagPaginationKeepVisibilityAndFilters(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "page-primary", IsAdmin: true})
	delegated := mustCreateUser(t, models.User{Username: "page-delegated", IsAdmin: true})
	owner := mustCreateUser(t, models.User{Username: "page-author"})
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityNotesView, authorization.CapabilityNotesViewHidden}); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 6, 6, 0, 0, 0, 0, shanghaiLocation())
	messages := []models.Message{
		{Content: "#精确 needle public", UserID: owner.ID, Username: owner.Username, Visibility: MessageVisibilityPublic, CreatedAt: day},
		{Content: "#精确 needle private", UserID: owner.ID, Username: owner.Username, Visibility: MessageVisibilityPrivate, Private: true, CreatedAt: day},
		{Content: "#精确 needle protected", UserID: primary.ID, Username: primary.Username, Visibility: MessageVisibilityPrivate, Private: true, CreatedAt: day},
		{Content: "#精确 needle deleted", UserID: owner.ID, Username: owner.Username, Visibility: MessageVisibilityPublic, CreatedAt: day, DeletedAt: &day},
		{Content: "#精确 needle tomorrow", UserID: owner.ID, Username: owner.Username, Visibility: MessageVisibilityPublic, CreatedAt: day.AddDate(0, 0, 1)},
		{Content: "#精确 other keyword", UserID: owner.ID, Username: owner.Username, Visibility: MessageVisibilityPublic, CreatedAt: day},
		{Content: "#精确 needle users", UserID: owner.ID, Username: owner.Username, Visibility: MessageVisibilityUsers, Private: true, CreatedAt: day},
		{Content: "#精确 needle excluded", UserID: owner.ID, Username: owner.Username, Visibility: MessageVisibilityPublic, CreatedAt: day},
	}
	if err := db.Create(&messages).Error; err != nil {
		t.Fatal(err)
	}
	date, keyword, tag := "2026-06-06", "needle", "精确"
	for _, test := range []struct {
		name   string
		viewer *uint
		want   []uint
	}{
		{name: "anonymous", want: []uint{messages[0].ID}},
		{name: "owner", viewer: &owner.ID, want: []uint{messages[6].ID, messages[1].ID, messages[0].ID}},
		{name: "delegated", viewer: &delegated.ID, want: []uint{messages[6].ID, messages[1].ID, messages[0].ID}},
		{name: "primary", viewer: &primary.ID, want: []uint{messages[6].ID, messages[2].ID, messages[1].ID, messages[0].ID}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, filterTag := range []*string{nil, &tag} {
				result, err := GetMessagesByPage(1, 10, test.viewer, nil, nil, &date, &keyword, filterTag, MessagePinScopeLatest, &messages[7].ID)
				if err != nil || result.Total != int64(len(test.want)) {
					t.Fatalf("list=%#v err=%v", result, err)
				}
				assertMessageIDs(t, result.Items, test.want...)
				for index, id := range test.want {
					location, err := LocateMessagePage(id, 2, test.viewer, nil, nil, &date, &keyword, filterTag, MessagePinScopeLatest, &messages[7].ID)
					if err != nil || location.Page != index/2+1 || location.Total != int64(len(test.want)) {
						t.Fatalf("locate %d=%#v err=%v", id, location, err)
					}
				}
				for _, message := range messages {
					allowed := false
					for _, id := range test.want {
						allowed = allowed || message.ID == id
					}
					if !allowed {
						if _, err := LocateMessagePage(message.ID, 2, test.viewer, nil, nil, &date, &keyword, filterTag, MessagePinScopeLatest, &messages[7].ID); err == nil {
							t.Fatalf("filtered/hidden message %d was located", message.ID)
						}
					}
				}
			}
		})
	}
	if _, err := LocateMessagePage(messages[0].ID, 10, nil, nil, &primary.Username, nil, nil, nil, MessagePinScopeLatest, nil); err == nil {
		t.Fatal("username filter ignored")
	}
	if _, err := LocateMessagePage(messages[0].ID, 10, nil, &primary.ID, &owner.Username, nil, nil, nil, MessagePinScopeLatest, nil); err == nil {
		t.Fatal("author ID must override username filter")
	}
	location, err := LocateMessagePage(messages[0].ID, 101, nil, nil, nil, nil, nil, nil, MessagePinScopeLatest, nil)
	if err != nil || location.PageSize != 10 {
		t.Fatalf("invalid page size not normalized: %#v err=%v", location, err)
	}
}

func TestMessageCursorBoundaryMatchesNullableDescendingOrder(t *testing.T) {
	db := setupUserServiceTestDB(t)
	base := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	for _, pin := range []interface{}{nil, false, true} {
		for _, pinAt := range []interface{}{nil, base, base.Add(time.Hour)} {
			for _, createdAt := range []interface{}{nil, base, base.Add(time.Minute)} {
				for range 2 {
					if err := db.Model(&models.Message{}).Create(map[string]interface{}{
						"content": "#nullable", "user_id": 42, "visibility": MessageVisibilityPublic,
						"pinned": pin, "personal_pinned": pin, "pinned_at": pinAt, "personal_pinned_at": pinAt, "created_at": createdAt,
					}).Error; err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, pinScope := range []string{MessagePinScopeLatest, MessagePinScopePersonal} {
			t.Run(dialect+"/"+pinScope, func(t *testing.T) {
				order := messagePinOrder(pinScope)
				if dialect == "postgres" {
					// SQLite supports explicit NULLS FIRST, allowing the PostgreSQL
					// comparison branch to be checked without a remote database.
					pin, _ := messagePinColumns(pinScope)
					order = strings.Replace(order, pin+" DESC", pin+" DESC NULLS FIRST", 1)
					order = strings.Replace(order, "created_at DESC", "created_at DESC NULLS FIRST", 1)
				}
				var ordered []messagePageCursor
				if err := db.Model(&models.Message{}).Select(messageCursorProjection(pinScope)).Order(order).Find(&ordered).Error; err != nil {
					t.Fatal(err)
				}
				for index, cursor := range ordered {
					for _, before := range []bool{false, true} {
						condition, args := messageCursorBoundary(cursor, pinScope, before, dialect)
						var count int64
						if err := db.Model(&models.Message{}).Where(condition, args...).Count(&count).Error; err != nil {
							t.Fatal(err)
						}
						want := len(ordered) - index - 1
						if before {
							want = index
						}
						if count != int64(want) {
							t.Fatalf("cursor index=%d before=%v count=%d want=%d", index, before, count, want)
						}
					}
				}
			})
		}
	}
}

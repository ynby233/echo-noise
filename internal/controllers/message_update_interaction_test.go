package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
)

func TestTaskUpdateResponsePreservesViewerInteraction(t *testing.T) {
	db, r, actor := setupMessagePublishTimeTest(t)
	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true}
	other := models.User{ID: 1602, Username: "other"}
	for _, user := range []*models.User{&primary, &other} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	*actor = primary.ID
	for _, tc := range []struct {
		name       string
		owner      uint
		visibility string
		want       bool
	}{
		{"own task", primary.ID, "private", true},
		{"public task", other.ID, "public", true},
		{"hidden task managed by primary", other.ID, "private", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message := models.Message{UserID: tc.owner, Content: "- [ ] task", Visibility: tc.visibility, Private: tc.visibility == "private"}
			if err := db.Create(&message).Error; err != nil {
				t.Fatal(err)
			}
			for _, content := range []string{"- [x] task", "- [ ] task"} {
				w := performMessageJSONRequest(r, http.MethodPut, "/messages/"+strconv.FormatUint(uint64(message.ID), 10), map[string]any{"content": content})
				assertMessageResponseCode(t, w, http.StatusOK, 1)
				var response struct {
					Data models.Message `json:"data"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response.Data.Content != content || response.Data.CanInteract != tc.want {
					t.Fatalf("updated content=%q can_interact=%v; want content=%q can_interact=%v", response.Data.Content, response.Data.CanInteract, content, tc.want)
				}
			}
		})
	}
}

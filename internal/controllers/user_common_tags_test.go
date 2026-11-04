package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
)

func TestUserCommonTagsSelfAPIAndPrivacy(t *testing.T) {
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "passwords.db"))
	db, router, owner, _ := setupCommentAccountTest(t)
	other := models.User{Username: "othertagowner", CommonTags: `["secretother"]`}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	router.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-User") == "owner" {
			c.Set("user_id", owner.ID)
		}
	})
	router.PUT("/user/update", UpdateUser)
	router.GET("/user", GetUserInfo)
	router.GET("/users/profile", GetUserProfile)
	request := func(method, path, body string, authenticated bool) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		if authenticated {
			req.Header.Set("X-Test-User", "owner")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		var result map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if result := request(http.MethodPut, "/user/update", `{"commonTags":["guest"]}`, false); result["code"] == float64(1) {
		t.Fatal("anonymous update succeeded")
	}
	body := `{"id":` + strconvFormatUint(other.ID) + `,"user_id":` + strconvFormatUint(other.ID) + `,"commonTags":[" #工作 ","工作","生活"]}`
	result := request(http.MethodPut, "/user/update?id="+strconvFormatUint(other.ID), body, true)
	if result["code"] != float64(1) {
		t.Fatalf("ordinary self update failed: %v", result)
	}
	data := result["data"].(map[string]any)
	if tags := data["commonTags"].([]any); len(tags) != 2 || tags[0] != "工作" {
		t.Fatalf("wrong canonical response: %v", tags)
	}
	repository.ClearUserCache()
	loaded := request(http.MethodGet, "/user", "", true)
	if tags := loaded["data"].(map[string]any)["commonTags"].([]any); len(tags) != 2 {
		t.Fatal("self reload lost tags")
	}
	for _, invalid := range []string{`{"commonTags":"bad"}`, `{"commonTags":[123]}`, `{"commonTags":["bad tag"]}`} {
		if result := request(http.MethodPut, "/user/update", invalid, true); result["code"] == float64(1) {
			t.Fatalf("accepted invalid input: %s", invalid)
		}
	}
	public := request(http.MethodGet, "/users/profile?id="+strconvFormatUint(other.ID), "", true)
	if public["code"] != float64(1) {
		t.Fatalf("public profile failed: %v", public)
	}
	if _, exposed := public["data"].(map[string]any)["commonTags"]; exposed {
		t.Fatal("public profile exposes tags")
	}
	var untouched models.User
	if err := db.First(&untouched, other.ID).Error; err != nil {
		t.Fatal(err)
	}
	if untouched.CommonTags != `["secretother"]` {
		t.Fatal("request target overrode authenticated owner")
	}
	encoded, err := json.Marshal([]models.User{untouched})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secretother") || strings.Contains(string(encoded), "commonTags") {
		t.Fatal("user list model exposes tags")
	}
	cleared := request(http.MethodPut, "/user/update", `{"commonTags":[]}`, true)
	if cleared["code"] != float64(1) || len(cleared["data"].(map[string]any)["commonTags"].([]any)) != 0 {
		t.Fatal("failed to clear tags")
	}
}

func TestUserCommonTagsSelfResponseKeepsPasswordPrivate(t *testing.T) {
	user := models.User{Password: "secretpassword", CommonTags: `["work"]`}
	encoded, err := json.Marshal(selfUserResponse(user))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secretpassword") || !strings.Contains(string(encoded), `"commonTags":["work"]`) {
		t.Fatalf("unexpected self response: %s", encoded)
	}
	if user.Password != "secretpassword" {
		t.Fatal("response mutated cached user")
	}
}

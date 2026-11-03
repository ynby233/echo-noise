package routers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAuthorSubscriptionRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SESSION_SECRET", "author-subscription-route-test-secret-32-bytes")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.AuthorSubscription{}, &models.SiteConfig{}); err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = previousDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	viewer := models.User{Username: "subscription-viewer"}
	author := models.User{Username: "subscription-author"}
	other := models.User{Username: "subscription-other"}
	for _, user := range []*models.User{&viewer, &author, &other} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := SetupRouter(nil)
	router.GET("/test-author-subscription-session/:id", func(c *gin.Context) {
		var user models.User
		if err := db.First(&user, c.Param("id")).Error; err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		session := sessions.Default(c)
		session.Set("user_id", user.ID)
		session.Set("username", user.Username)
		session.Set("is_admin", user.IsAdmin)
		if err := session.Save(); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	login := func(userID uint) []*http.Cookie {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/test-author-subscription-session/%d", userID), nil))
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("seed session: %d %s", recorder.Code, recorder.Body.String())
		}
		return recorder.Result().Cookies()
	}
	cookies := login(viewer.ID)
	request := func(method, id string, authCookies []*http.Cookie, want int) *httptest.ResponseRecorder {
		t.Helper()
		// A supplied identity must never override the authenticated viewer.
		body := fmt.Sprintf(`{"subscriber_user_id":%d,"author_id":%d}`, other.ID, other.ID)
		req := httptest.NewRequest(method, "/api/authors/"+id+"/subscription", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for _, cookie := range authCookies {
			req.AddCookie(cookie)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if recorder.Code != want {
			t.Fatalf("%s author %s: status=%d want=%d body=%s", method, id, recorder.Code, want, recorder.Body.String())
		}
		return recorder
	}
	assertState := func(recorder *httptest.ResponseRecorder, subscribed bool) {
		t.Helper()
		var response struct {
			Code int `json:"code"`
			Data struct {
				AuthorID   uint `json:"author_id"`
				Subscribed bool `json:"subscribed"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Code != 1 || response.Data.AuthorID != author.ID || response.Data.Subscribed != subscribed {
			t.Fatalf("unexpected subscription response: %s", recorder.Body.String())
		}
	}
	authorID := fmt.Sprint(author.ID)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method+" validation", func(t *testing.T) {
			request(method, authorID, nil, http.StatusUnauthorized)
			request(method, "999999", cookies, http.StatusNotFound)
			request(method, fmt.Sprint(viewer.ID), cookies, http.StatusBadRequest)
			for _, id := range []string{"0", "-1", "abc", "18446744073709551616"} {
				request(method, id, cookies, http.StatusBadRequest)
			}
		})
	}
	assertState(request(http.MethodGet, authorID, cookies, http.StatusOK), false)
	assertState(request(http.MethodPut, authorID, cookies, http.StatusOK), true)
	var original models.AuthorSubscription
	if err := db.First(&original).Error; err != nil {
		t.Fatal(err)
	}
	assertState(request(http.MethodPut, authorID, cookies, http.StatusOK), true)
	var rows []models.AuthorSubscription
	if err := db.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].SubscriberUserID != viewer.ID || rows[0].AuthorUserID != author.ID || !rows[0].CreatedAt.Equal(original.CreatedAt) {
		t.Fatalf("repeated PUT or supplied identity changed subscriptions: %+v", rows)
	}
	assertState(request(http.MethodGet, authorID, cookies, http.StatusOK), true)
	otherCookies := login(other.ID)
	assertState(request(http.MethodGet, authorID, otherCookies, http.StatusOK), false)
	assertState(request(http.MethodDelete, authorID, otherCookies, http.StatusOK), false)
	assertState(request(http.MethodGet, authorID, cookies, http.StatusOK), true)
	assertState(request(http.MethodDelete, authorID, cookies, http.StatusOK), false)
	assertState(request(http.MethodDelete, authorID, cookies, http.StatusOK), false)
	assertState(request(http.MethodGet, authorID, cookies, http.StatusOK), false)
	if err := db.Delete(&viewer).Error; err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		request(method, authorID, cookies, http.StatusUnauthorized)
	}
}

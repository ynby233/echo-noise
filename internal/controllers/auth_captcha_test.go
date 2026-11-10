package controllers

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/models"
)

func setupAuthCaptchaTest(t *testing.T) *gin.Engine {
	t.Helper()
	db, r, _, _ := setupCommentAccountTest(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	// Every concurrent request must use the same SQLite :memory: database.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Setting{AllowRegistration: true}).Error; err != nil {
		t.Fatal(err)
	}
	r.GET("/captcha", GetCaptcha)
	r.POST("/register", Register)
	// These test-only routes use the same signing middleware as /captcha.
	r.GET("/test/legacy", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Delete("captcha_id")
		session.Set("captcha_code", "ABCDE")
		session.Set("captcha_exp", time.Now().Add(2*time.Minute).Unix())
		if err := session.Save(); err != nil {
			c.String(http.StatusInternalServerError, "%v", err)
			return
		}
		c.Status(http.StatusOK)
	})
	r.GET("/test/session", func(c *gin.Context) {
		session := sessions.Default(c)
		c.JSON(http.StatusOK, gin.H{
			"id":   session.Get("captcha_id"),
			"code": session.Get("captcha_code"),
			"exp":  session.Get("captcha_exp"),
		})
	})
	return r
}

func authCaptchaRequest(r http.Handler, method, path, body, cookie string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		// Preserve the original signed value, without accepting later Set-Cookie headers.
		req.Header.Set("Cookie", strings.SplitN(cookie, ";", 2)[0])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func authCaptchaRegisterBody(id, answer string) string {
	// Nonempty for binding, but blank after service trimming: no password vault access.
	body, _ := json.Marshal(map[string]string{
		"username": " \t ", "password": "captcha-test-unused", "captcha_id": id, "captcha": answer,
	})
	return string(body)
}

func assertAuthCaptchaMessage(t *testing.T, w *httptest.ResponseRecorder, want string) {
	t.Helper()
	var response struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &response) != nil {
		t.Fatalf("unexpected response: status=%d body=%s", w.Code, w.Body.String())
	}
	if response.Code != 0 || response.Message != want {
		t.Fatalf("response=%s, want failure message %q", w.Body.String(), want)
	}
}

func authCaptchaSVGAnswer(t *testing.T, svg string) string {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader(svg))
	var answer string
	var root bool
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("invalid SVG: %v", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "svg" {
			root = start.Name.Space == "http://www.w3.org/2000/svg"
		}
		if start.Name.Local == "text" {
			if err := decoder.DecodeElement(&answer, &start); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !root || len(answer) != 5 || strings.Trim(answer, "ABCDEFGHJKLMNPQRSTUVWXYZ23456789") != "" {
		t.Fatalf("unexpected captcha SVG: %s", svg)
	}
	return answer
}

func authCaptchaSession(t *testing.T, r http.Handler, cookie string) string {
	t.Helper()
	w := authCaptchaRequest(r, http.MethodGet, "/test/session", "", cookie)
	var session struct {
		ID   string `json:"id"`
		Code any    `json:"code"`
		Exp  any    `json:"exp"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.ID == "" || session.Code != nil || session.Exp != nil {
		t.Fatalf("expected signed ID-only session, got %s", w.Body.String())
	}
	t.Cleanup(func() { deleteCaptcha(session.ID) })
	return session.ID
}

func authCaptchaJSON(t *testing.T, r http.Handler) (string, string) {
	t.Helper()
	before := time.Now().Unix()
	w := authCaptchaRequest(r, http.MethodGet, "/captcha?json=1", "", "")
	after := time.Now().Unix()
	var response struct {
		Code int `json:"code"`
		Data struct {
			ID        string `json:"captcha_id"`
			SVG       string `json:"svg"`
			ExpiresIn int    `json:"expires_in"`
		} `json:"data"`
	}
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("unexpected JSON response: %d %v", w.Code, w.Header())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != 1 || response.Data.ID == "" || response.Data.ExpiresIn != 120 {
		t.Fatalf("invalid captcha contract: %s", w.Body.String())
	}
	if w.Header().Get("Set-Cookie") != "" {
		t.Fatal("JSON captcha unexpectedly requires a cookie")
	}
	id := response.Data.ID
	t.Cleanup(func() { deleteCaptcha(id) })
	answer := authCaptchaSVGAnswer(t, response.Data.SVG)
	item, ok := getCaptcha(id)
	if !ok || item.Code != answer || item.Exp < before+120 || item.Exp > after+120 {
		t.Fatalf("server captcha does not match SVG/120-second lifetime: %+v, found=%v", item, ok)
	}
	return id, answer
}

func TestAuthCaptchaSignedCookieReplay(t *testing.T) {
	r := setupAuthCaptchaTest(t)
	w := authCaptchaRequest(r, http.MethodGet, "/captcha", "", "")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("unexpected image response: %d %v", w.Code, w.Header())
	}
	answer := authCaptchaSVGAnswer(t, w.Body.String())
	originalSetCookie := w.Header().Get("Set-Cookie")
	if originalSetCookie == "" {
		t.Fatal("missing signed session cookie")
	}
	id := authCaptchaSession(t, r, originalSetCookie)
	assertAuthCaptchaMessage(t, authCaptchaRequest(r, http.MethodPost, "/register", authCaptchaRegisterBody("", "wrong"), originalSetCookie), "验证码不正确")
	assertAuthCaptchaMessage(t, authCaptchaRequest(r, http.MethodPost, "/register", authCaptchaRegisterBody("", " \t"+strings.ToLower(answer)+"\n"), originalSetCookie), models.UsernameOrPasswordCannotBeEmptyMessage)
	assertAuthCaptchaMessage(t, authCaptchaRequest(r, http.MethodPost, "/register", authCaptchaRegisterBody("", answer), originalSetCookie), "验证码已过期")
	if _, ok := getCaptcha(id); ok {
		t.Fatal("successful validation left captcha available for replay")
	}
}

func TestAuthCaptchaLegacySignedCookieRequiresRefresh(t *testing.T) {
	r := setupAuthCaptchaTest(t)
	legacy := authCaptchaRequest(r, http.MethodGet, "/test/legacy", "", "")
	originalSetCookie := legacy.Header().Get("Set-Cookie")
	if legacy.Code != http.StatusOK || originalSetCookie == "" {
		t.Fatalf("cannot create legacy signed cookie: %s", legacy.Body.String())
	}
	for range 2 {
		assertAuthCaptchaMessage(t, authCaptchaRequest(r, http.MethodPost, "/register", authCaptchaRegisterBody("", "ABCDE"), originalSetCookie), "验证码已过期")
	}
	fresh := authCaptchaRequest(r, http.MethodGet, "/captcha", "", originalSetCookie)
	freshCookie := fresh.Header().Get("Set-Cookie")
	if fresh.Code != http.StatusOK || freshCookie == "" {
		t.Fatalf("captcha refresh failed: %s", fresh.Body.String())
	}
	authCaptchaSession(t, r, freshCookie)
	answer := authCaptchaSVGAnswer(t, fresh.Body.String())
	assertAuthCaptchaMessage(t, authCaptchaRequest(r, http.MethodPost, "/register", authCaptchaRegisterBody("", answer), freshCookie), models.UsernameOrPasswordCannotBeEmptyMessage)
	assertAuthCaptchaMessage(t, authCaptchaRequest(r, http.MethodPost, "/register", authCaptchaRegisterBody("", "ABCDE"), originalSetCookie), "验证码已过期")
}

func TestAuthCaptchaJSONContractAndConsumption(t *testing.T) {
	r := setupAuthCaptchaTest(t)
	id, answer := authCaptchaJSON(t, r)
	assertAuthCaptchaMessage(t, authCaptchaRequest(r, http.MethodPost, "/register", authCaptchaRegisterBody(id, "wrong"), ""), "验证码不正确")
	assertAuthCaptchaMessage(t, authCaptchaRequest(r, http.MethodPost, "/register", authCaptchaRegisterBody(" \t"+id+" ", " \t"+strings.ToLower(answer)+"\n"), ""), models.UsernameOrPasswordCannotBeEmptyMessage)
	assertAuthCaptchaMessage(t, authCaptchaRequest(r, http.MethodPost, "/register", authCaptchaRegisterBody(id, answer), ""), "验证码已过期")
}

func TestAuthCaptchaExpired(t *testing.T) {
	r := setupAuthCaptchaTest(t)
	for _, cookieMode := range []bool{false, true} {
		var id, answer, cookie string
		if cookieMode {
			w := authCaptchaRequest(r, http.MethodGet, "/captcha", "", "")
			cookie = w.Header().Get("Set-Cookie")
			id = authCaptchaSession(t, r, cookie)
			answer = authCaptchaSVGAnswer(t, w.Body.String())
		} else {
			id, answer = authCaptchaJSON(t, r)
		}
		setCaptcha(id, answer, time.Now().Add(-time.Second).Unix())
		requestID := id
		if cookieMode {
			requestID = ""
		}
		assertAuthCaptchaMessage(t, authCaptchaRequest(r, http.MethodPost, "/register", authCaptchaRegisterBody(requestID, answer), cookie), "验证码已过期")
		captchaStore.Lock()
		_, exists := captchaStore.m[id]
		captchaStore.Unlock()
		if exists {
			t.Fatal("expired captcha was not removed during consumption")
		}
	}
}

func TestAuthCaptchaConcurrentConsume(t *testing.T) {
	id := newCaptchaID()
	setCaptcha(id, "AbCdE", time.Now().Add(time.Minute).Unix())
	t.Cleanup(func() { deleteCaptcha(id) })
	const count = 16
	start := make(chan struct{})
	results := make(chan error, count)
	var ready sync.WaitGroup
	ready.Add(count)
	for range count {
		go func() {
			ready.Done()
			<-start
			results <- consumeCaptcha(id, " \tabcde\n")
		}()
	}
	ready.Wait()
	close(start)
	successes := 0
	for range count {
		if err := <-results; err == nil {
			successes++
		} else if err.Error() != "验证码已过期" {
			t.Errorf("unexpected consume error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful consumers=%d, want 1", successes)
	}
}

func TestAuthCaptchaConcurrentJSONRegister(t *testing.T) {
	r := setupAuthCaptchaTest(t)
	id, answer := authCaptchaJSON(t, r)
	body := []byte(authCaptchaRegisterBody(id, answer))
	const count = 16
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, count)
	var ready sync.WaitGroup
	ready.Add(count)
	for range count {
		go func() {
			// Request, body reader and recorder belong exclusively to this goroutine.
			req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			ready.Done()
			<-start
			r.ServeHTTP(w, req)
			results <- w
		}()
	}
	ready.Wait()
	close(start)
	passed := 0
	for range count {
		w := <-results
		var response struct {
			Message string `json:"msg"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Errorf("invalid register response: %s", w.Body.String())
			continue
		}
		if response.Message == models.UsernameOrPasswordCannotBeEmptyMessage {
			passed++
			assertAuthCaptchaMessage(t, w, models.UsernameOrPasswordCannotBeEmptyMessage)
		} else {
			assertAuthCaptchaMessage(t, w, "验证码已过期")
		}
	}
	if passed != 1 {
		t.Fatalf("requests passing captcha=%d, want 1", passed)
	}
}

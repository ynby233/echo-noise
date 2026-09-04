package pkg

import (
	"crypto/tls"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func TestSecureCookieFollowsRequestTransportAndExplicitOverride(t *testing.T) {
	tests := []struct {
		name           string
		cookieSecure   string
		requestTLS     bool
		forwardedProto string
		want           bool
	}{
		{name: "plain HTTP", want: false},
		{name: "direct HTTPS", requestTLS: true, want: true},
		{name: "proxied HTTPS", forwardedProto: "https", want: true},
		{name: "proxied HTTPS chain", forwardedProto: "https, http", want: true},
		{name: "forced secure", cookieSecure: "true", want: true},
		{name: "forced insecure", cookieSecure: "false", requestTLS: true, forwardedProto: "https", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("COOKIE_SECURE", test.cookieSecure)
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
			if test.requestTLS {
				context.Request.TLS = &tls.ConnectionState{}
			}
			if test.forwardedProto != "" {
				context.Request.Header.Set("X-Forwarded-Proto", test.forwardedProto)
			}
			if got := secureCookieForRequest(context); got != test.want {
				t.Fatalf("secureCookieForRequest() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestProductionSessionCookieWorksOverPlainHTTP(t *testing.T) {
	t.Setenv("GIN_MODE", "release")
	t.Setenv("APP_ENV", "production")
	t.Setenv("COOKIE_SECURE", "")
	t.Setenv("SESSION_SECRET", "0123456789abcdef0123456789abcdef")

	router := gin.New()
	InitSession(router)
	router.POST("/login", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", uint(1))
		if err := session.Save(); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	router.GET("/user", func(c *gin.Context) {
		if sessions.Default(c).Get("user_id") == nil {
			c.Status(http.StatusUnauthorized)
			return
		}
		c.Status(http.StatusNoContent)
	})
	router.POST("/logout", func(c *gin.Context) {
		if err := ClearUserSession(c); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})

	server := httptest.NewServer(router)
	defer server.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	loginResponse, err := client.Post(server.URL+"/login", "application/json", nil)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer loginResponse.Body.Close()
	if loginResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("login status = %d", loginResponse.StatusCode)
	}
	if setCookie := loginResponse.Header.Get("Set-Cookie"); strings.Contains(strings.ToLower(setCookie), "; secure") {
		t.Fatalf("plain HTTP response emitted a Secure session cookie: %s", setCookie)
	}

	userResponse, err := client.Get(server.URL + "/user")
	if err != nil {
		t.Fatalf("user request: %v", err)
	}
	defer userResponse.Body.Close()
	if userResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("plain HTTP session was not returned by the client: status = %d", userResponse.StatusCode)
	}

	logoutResponse, err := client.Post(server.URL+"/logout", "application/json", nil)
	if err != nil {
		t.Fatalf("logout request: %v", err)
	}
	defer logoutResponse.Body.Close()
	if logoutResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("logout status = %d", logoutResponse.StatusCode)
	}
	if cookies := jar.Cookies(logoutResponse.Request.URL); len(cookies) != 0 {
		t.Fatalf("logout left session cookies in the client jar: %#v", cookies)
	}

	afterLogoutResponse, err := client.Get(server.URL + "/user")
	if err != nil {
		t.Fatalf("user request after logout: %v", err)
	}
	defer afterLogoutResponse.Body.Close()
	if afterLogoutResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("session remained authenticated after logout: status = %d", afterLogoutResponse.StatusCode)
	}
}

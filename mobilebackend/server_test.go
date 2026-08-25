package mobilebackend

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
)

func TestFreshAndroidStandaloneRequiresOwnerSetupBeforeNormalAPIs(t *testing.T) {
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	workDirectory := t.TempDir()
	t.Setenv("SESSION_SECRET", "android-standalone-test-session-secret")
	t.Setenv("COOKIE_SECURE", "false")
	t.Setenv("DB_PATH", filepath.Join(workDirectory, "data", "noise.db"))

	if err := Start(workDirectory); err != nil {
		t.Fatalf("start Android backend: %v", err)
	}
	t.Cleanup(func() {
		Stop()
		if database.DB != nil {
			if sqlDB, err := database.DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		database.DB = nil
		models.SetDB(nil)
		_ = os.Chdir(originalWorkingDirectory)
	})

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 2 * time.Second, Jar: jar}
	baseURL := "http://127.0.0.1:1314"
	deadline := time.Now().Add(15 * time.Second)
	for {
		response, requestErr := client.Get(baseURL + "/api/setup/status")
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("Android setup endpoint did not become ready: %v", requestErr)
		}
		time.Sleep(100 * time.Millisecond)
	}

	response, err := client.Get(baseURL + "/api/messages")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("normal API before setup status=%d, want %d", response.StatusCode, http.StatusPreconditionRequired)
	}

	payload, _ := json.Marshal(map[string]string{
		"username":         "device_owner",
		"password":         "strong-password",
		"confirm_password": "strong-password",
	})
	response, err = client.Post(baseURL+"/api/setup/owner", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("owner setup status=%d", response.StatusCode)
	}
	if len(jar.Cookies(response.Request.URL)) == 0 {
		t.Fatal("owner setup did not establish a WebView login session")
	}

	response, err = client.Get(baseURL + "/api/messages")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("normal API after setup status=%d, want %d", response.StatusCode, http.StatusOK)
	}

	var owner models.User
	if err := database.DB.First(&owner, models.PrimaryAdminUserID).Error; err != nil || !owner.IsAdmin || owner.Username != "device_owner" {
		t.Fatalf("stored owner=%#v err=%v", owner, err)
	}
}

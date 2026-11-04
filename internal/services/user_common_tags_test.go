package services

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
)

func TestNormalizeCommonTags(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   []string
		want    []string
		invalid bool
	}{
		{name: "empty", want: []string{}},
		{name: "trim deduplicate preserve order", input: []string{" 工作 ", "#生活", "工作", "", "  ", "Go123"}, want: []string{"工作", "生活", "Go123"}},
		{name: "space", input: []string{"two words"}, invalid: true},
		{name: "punctuation", input: []string{"bad-tag"}, invalid: true},
		{name: "symbol", input: []string{"tag😀"}, invalid: true},
		{name: "embedded hash", input: []string{"a#b"}, invalid: true},
		{name: "media query", input: []string{"song?id=123"}, invalid: true},
		{name: "control", input: []string{"a\x00b"}, invalid: true},
		{name: "too long", input: []string{strings.Repeat("中", 65)}, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeCommonTags(tc.input)
			if (err != nil) != tc.invalid {
				t.Fatalf("validation error = %v", err)
			}
			if !tc.invalid && !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	values := make([]string, 51)
	for i := range values {
		values[i] = fmt.Sprintf("tag%d", i)
	}
	if _, err := NormalizeCommonTags(values); err == nil {
		t.Fatal("expected tag count limit")
	}
	for _, stored := range []string{"", "null", "broken", `["bad tag"]`} {
		if got := UserCommonTags(stored); got == nil || len(got) != 0 {
			t.Fatalf("legacy value %q: %v", stored, got)
		}
	}
}

func TestUserCommonTagsProfilePersistence(t *testing.T) {
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "passwords.db"))
	db := setupUserServiceTestDB(t)
	user := mustCreateUser(t, models.User{Username: "tagowner", Password: "original", CommonTags: `["old"]`})
	other := mustCreateUser(t, models.User{Username: "otherowner", Password: "other", CommonTags: `["private"]`})
	tags := []string{" #工作 ", "工作", "生活"}
	if err := UpdateUser(user, dto.UserInfoDto{CommonTags: &tags}); err != nil {
		t.Fatal(err)
	}
	repository.ClearUserCache()
	fresh, err := repository.GetUserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.CommonTags != `["工作","生活"]` || fresh.Password != "original" || fresh.IsAdmin {
		t.Fatalf("unexpected saved profile: %v", UserCommonTags(fresh.CommonTags))
	}
	if err := UpdateUser(fresh, dto.UserInfoDto{Description: "new signature"}); err != nil {
		t.Fatal(err)
	}
	invalid := []string{"not valid"}
	if err := UpdateUser(fresh, dto.UserInfoDto{Description: "must not save", CommonTags: &invalid}); err == nil {
		t.Fatal("expected invalid format to fail")
	}
	var loaded models.User
	if err := db.First(&loaded, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if loaded.Description != "new signature" || loaded.CommonTags != `["工作","生活"]` {
		t.Fatal("omitted/invalid tags modified saved profile")
	}
	empty := []string{}
	if err := UpdateUser(fresh, dto.UserInfoDto{CommonTags: &empty}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&loaded, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if loaded.CommonTags != `[]` {
		t.Fatalf("clear did not persist: %q", loaded.CommonTags)
	}
	var untouched models.User
	if err := db.First(&untouched, other.ID).Error; err != nil {
		t.Fatal(err)
	}
	if untouched.CommonTags != `["private"]` {
		t.Fatal("changed another account")
	}
}

package services

import (
	"sort"
	"testing"

	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
)

func TestContentReadScopeSeparatesHiddenReadsFromNormalInteractions(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "scope-primary", IsAdmin: true})
	delegated := mustCreateUser(t, models.User{Username: "scope-delegated", IsAdmin: true})
	delegatedWithHidden := mustCreateUser(t, models.User{Username: "scope-hidden", IsAdmin: true})
	ordinary := mustCreateUser(t, models.User{Username: "scope-ordinary"})
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegatedWithHidden.ID, []authorization.Capability{authorization.CapabilityContentViewHidden}); err != nil {
		t.Fatalf("grant hidden-content read: %v", err)
	}

	ordinaryPrivate := models.Message{Content: "ordinary private", UserID: ordinary.ID, Visibility: MessageVisibilityPrivate, Private: true}
	primaryPrivate := models.Message{Content: "primary private", UserID: primary.ID, Visibility: MessageVisibilityPrivate, Private: true}
	primaryPublic := models.Message{Content: "primary public", UserID: primary.ID, Visibility: MessageVisibilityPublic}
	for _, message := range []*models.Message{&ordinaryPrivate, &primaryPrivate, &primaryPublic} {
		if err := db.Create(message).Error; err != nil {
			t.Fatalf("create %q: %v", message.Content, err)
		}
	}

	tests := []struct {
		name                   string
		actorID                *uint
		message                models.Message
		wantRead, wantInteract bool
	}{
		{name: "guest reads public", message: primaryPublic, wantRead: true},
		{name: "ordinary owner reads and interacts with private", actorID: &ordinary.ID, message: ordinaryPrivate, wantRead: true, wantInteract: true},
		{name: "delegated without grant cannot read hidden", actorID: &delegated.ID, message: ordinaryPrivate},
		{name: "delegated grant reads ordinary hidden without interacting", actorID: &delegatedWithHidden.ID, message: ordinaryPrivate, wantRead: true},
		{name: "delegated grant cannot read primary hidden", actorID: &delegatedWithHidden.ID, message: primaryPrivate},
		{name: "delegated grant still reads primary public normally", actorID: &delegatedWithHidden.ID, message: primaryPublic, wantRead: true, wantInteract: true},
		{name: "primary reads ordinary hidden without interacting", actorID: &primary.ID, message: ordinaryPrivate, wantRead: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, err := ResolveContentReadScope(db, tt.actorID)
			if err != nil {
				t.Fatalf("resolve scope: %v", err)
			}
			if got := scope.CanReadMessage(tt.message); got != tt.wantRead {
				t.Fatalf("CanReadMessage=%v, want %v", got, tt.wantRead)
			}
			if got := scope.CanInteractWithMessage(tt.message); got != tt.wantInteract {
				t.Fatalf("CanInteractWithMessage=%v, want %v", got, tt.wantInteract)
			}
		})
	}
}

func TestContentReadScopeAppliesTheSameRulesToMessageQueries(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "query-primary", IsAdmin: true})
	delegated := mustCreateUser(t, models.User{Username: "query-delegated", IsAdmin: true})
	delegatedWithHidden := mustCreateUser(t, models.User{Username: "query-hidden", IsAdmin: true})
	ordinary := mustCreateUser(t, models.User{Username: "query-ordinary"})
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegatedWithHidden.ID, []authorization.Capability{authorization.CapabilityContentViewHidden}); err != nil {
		t.Fatalf("grant hidden-content read: %v", err)
	}
	messages := []models.Message{
		{Content: "ordinary public", UserID: ordinary.ID, Visibility: MessageVisibilityPublic},
		{Content: "ordinary private", UserID: ordinary.ID, Visibility: MessageVisibilityPrivate, Private: true},
		{Content: "primary public", UserID: primary.ID, Visibility: MessageVisibilityPublic},
		{Content: "primary private", UserID: primary.ID, Visibility: MessageVisibilityPrivate, Private: true},
	}
	if err := db.Create(&messages).Error; err != nil {
		t.Fatalf("create messages: %v", err)
	}

	tests := []struct {
		name    string
		actorID *uint
		want    []string
	}{
		{name: "guest", want: []string{"ordinary public", "primary public"}},
		{name: "delegated without grant", actorID: &delegated.ID, want: []string{"ordinary public", "primary public"}},
		{name: "delegated with grant", actorID: &delegatedWithHidden.ID, want: []string{"ordinary private", "ordinary public", "primary public"}},
		{name: "primary", actorID: &primary.ID, want: []string{"ordinary private", "ordinary public", "primary private", "primary public"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, err := ResolveContentReadScope(db, tt.actorID)
			if err != nil {
				t.Fatalf("resolve scope: %v", err)
			}
			var gotRows []models.Message
			if err := scope.ApplyMessageVisibility(database.DB.Model(&models.Message{})).Find(&gotRows).Error; err != nil {
				t.Fatalf("query messages: %v", err)
			}
			got := make([]string, 0, len(gotRows))
			for _, row := range gotRows {
				got = append(got, row.Content)
			}
			sort.Strings(got)
			if !sameStringSlice(got, tt.want) {
				t.Fatalf("visible messages=%#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestContentReadScopeProtectsPrimaryHiddenComments(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "comment-primary", IsAdmin: true})
	delegated := mustCreateUser(t, models.User{Username: "comment-delegated", IsAdmin: true})
	ordinary := mustCreateUser(t, models.User{Username: "comment-ordinary"})
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityContentViewHidden}); err != nil {
		t.Fatalf("grant hidden-content read: %v", err)
	}
	message := models.Message{Content: "ordinary public", UserID: ordinary.ID, Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	ordinaryID := ordinary.ID
	primaryID := primary.ID
	ordinaryPrivate := models.Comment{ID: 101, MessageID: message.ID, UserID: &ordinaryID, Visibility: MessageVisibilityPrivate}
	primaryPrivate := models.Comment{ID: 102, MessageID: message.ID, UserID: &primaryID, Visibility: MessageVisibilityPrivate}
	commentMap := CommentMap([]models.Comment{ordinaryPrivate, primaryPrivate})

	scope, err := ResolveContentReadScope(db, &delegated.ID)
	if err != nil {
		t.Fatalf("resolve scope: %v", err)
	}
	if !scope.CanReadComment(message, ordinaryPrivate, commentMap) {
		t.Fatal("delegated hidden-content reader must read ordinary hidden comment")
	}
	if scope.CanReadComment(message, primaryPrivate, commentMap) {
		t.Fatal("delegated hidden-content reader must not read primary hidden comment")
	}
}

func TestContentReadScopeSeparatesHiddenCommentReadsFromReplies(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "comment-interaction-primary", IsAdmin: true})
	delegated := mustCreateUser(t, models.User{Username: "comment-interaction-delegated", IsAdmin: true})
	ordinary := mustCreateUser(t, models.User{Username: "comment-interaction-ordinary"})
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityContentViewHidden}); err != nil {
		t.Fatalf("grant hidden-content read: %v", err)
	}
	message := models.Message{Content: "ordinary public", UserID: ordinary.ID, Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	ordinaryID := ordinary.ID
	publicComment := models.Comment{ID: 201, MessageID: message.ID, UserID: &ordinaryID, Visibility: MessageVisibilityPublic}
	privateComment := models.Comment{ID: 202, MessageID: message.ID, UserID: &ordinaryID, Visibility: MessageVisibilityPrivate}
	commentMap := CommentMap([]models.Comment{publicComment, privateComment})

	scope, err := ResolveContentReadScope(db, &delegated.ID)
	if err != nil {
		t.Fatalf("resolve scope: %v", err)
	}
	if !scope.CanReadComment(message, privateComment, commentMap) {
		t.Fatal("hidden-content reader must be able to read ordinary hidden comment")
	}
	if scope.CanInteractWithComment(message, privateComment, commentMap) {
		t.Fatal("hidden-content read must not allow replying to a normally hidden comment")
	}
	if !scope.CanInteractWithComment(message, publicComment, commentMap) {
		t.Fatal("normally visible comment must remain replyable")
	}
}

func TestGuestbookReadIgnoresDelegatedHiddenContentCapability(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "guestbook-primary", IsAdmin: true})
	delegated := mustCreateUser(t, models.User{Username: "guestbook-delegated", IsAdmin: true})
	delegatedWithHidden := mustCreateUser(t, models.User{Username: "guestbook-hidden", IsAdmin: true})
	ordinary := mustCreateUser(t, models.User{Username: "guestbook-ordinary"})
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegatedWithHidden.ID, []authorization.Capability{authorization.CapabilityContentViewHidden}); err != nil {
		t.Fatalf("grant hidden-content read: %v", err)
	}
	guestbook := models.Message{Content: models.CanonicalGuestbookContent, UserID: primary.ID, IsGuestbook: true, Visibility: MessageVisibilityPublic}
	ordinaryID := ordinary.ID
	delegatedID := delegated.ID
	delegatedWithHiddenID := delegatedWithHidden.ID
	primaryID := primary.ID
	comments := []models.Comment{
		{ID: 401, MessageID: 1, UserID: &ordinaryID, Content: "ordinary private", Visibility: "private"},
		{ID: 402, MessageID: 1, UserID: &delegatedID, Content: "delegated private", Visibility: "private"},
		{ID: 403, MessageID: 1, UserID: &primaryID, Content: "primary private", Visibility: "private"},
		{ID: 404, MessageID: 1, UserID: &delegatedWithHiddenID, Content: "hidden delegated private", Visibility: "private"},
	}
	commentMap := CommentMap(comments)

	withoutHidden, err := ResolveContentReadScope(db, &delegated.ID)
	if err != nil {
		t.Fatalf("resolve delegated scope: %v", err)
	}
	withHidden, err := ResolveContentReadScope(db, &delegatedWithHidden.ID)
	if err != nil {
		t.Fatalf("resolve hidden delegated scope: %v", err)
	}
	primaryScope, err := ResolveContentReadScope(db, &primary.ID)
	if err != nil {
		t.Fatalf("resolve primary scope: %v", err)
	}
	for _, scope := range []ContentReadScope{withoutHidden, withHidden} {
		if scope.CanReadComment(guestbook, comments[0], commentMap) {
			t.Fatal("delegated administrators must not read another user's hidden guestbook entry")
		}
	}
	if !withoutHidden.CanReadComment(guestbook, comments[1], commentMap) || !withHidden.CanReadComment(guestbook, comments[3], commentMap) {
		t.Fatal("delegated administrators must retain their own private guestbook visibility")
	}
	if withoutHidden.CanReadComment(guestbook, comments[2], commentMap) || withHidden.CanReadComment(guestbook, comments[2], commentMap) {
		t.Fatal("delegated administrators must not read primary-admin hidden guestbook entries")
	}
	for _, comment := range comments {
		if !primaryScope.CanReadComment(guestbook, comment, commentMap) {
			t.Fatalf("primary administrator must read all guestbook entries, comment=%d", comment.ID)
		}
	}
}

func TestAuthorizeMessageMutationRequiresReadScopeActionGrantAndProtection(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "mutation-primary", IsAdmin: true})
	withoutHidden := mustCreateUser(t, models.User{Username: "mutation-without-hidden", IsAdmin: true})
	withoutEdit := mustCreateUser(t, models.User{Username: "mutation-without-edit", IsAdmin: true})
	allowed := mustCreateUser(t, models.User{Username: "mutation-allowed", IsAdmin: true})
	ordinary := mustCreateUser(t, models.User{Username: "mutation-ordinary"})
	authorizer := authorization.New(db)
	if err := authorizer.ReplaceGrants(primary.ID, withoutHidden.ID, []authorization.Capability{authorization.CapabilityNotesEdit}); err != nil {
		t.Fatalf("grant edit only: %v", err)
	}
	if err := authorizer.ReplaceGrants(primary.ID, withoutEdit.ID, []authorization.Capability{authorization.CapabilityContentViewHidden}); err != nil {
		t.Fatalf("grant hidden read only: %v", err)
	}
	if err := authorizer.ReplaceGrants(primary.ID, allowed.ID, []authorization.Capability{authorization.CapabilityContentViewHidden, authorization.CapabilityNotesEdit}); err != nil {
		t.Fatalf("grant hidden read and edit: %v", err)
	}
	ordinaryPrivate := models.Message{Content: "ordinary hidden", UserID: ordinary.ID, Visibility: MessageVisibilityPrivate, Private: true}
	primaryPrivate := models.Message{Content: "primary hidden", UserID: primary.ID, Visibility: MessageVisibilityPrivate, Private: true}

	if decision := AuthorizeMessageMutation(db, withoutHidden.ID, ordinaryPrivate, authorization.CapabilityNotesEdit); decision.Allowed || decision.Reason != authorization.DenialContentNotReadable {
		t.Fatalf("edit grant without hidden read must be denied by read scope: %#v", decision)
	}
	if decision := AuthorizeMessageMutation(db, withoutEdit.ID, ordinaryPrivate, authorization.CapabilityNotesEdit); decision.Allowed || decision.Reason != authorization.DenialMissingGrant {
		t.Fatalf("hidden read without edit must be denied by action grant: %#v", decision)
	}
	if decision := AuthorizeMessageMutation(db, allowed.ID, ordinaryPrivate, authorization.CapabilityNotesEdit); !decision.Allowed {
		t.Fatalf("hidden read plus edit must allow ordinary hidden target: %#v", decision)
	}
	if decision := AuthorizeMessageMutation(db, allowed.ID, primaryPrivate, authorization.CapabilityNotesEdit); decision.Allowed || decision.Reason != authorization.DenialContentNotReadable {
		t.Fatalf("delegated admin must not discover or mutate primary hidden target: %#v", decision)
	}
	if decision := AuthorizeMessageMutation(db, primary.ID, ordinaryPrivate, authorization.CapabilityNotesEdit); !decision.Allowed {
		t.Fatalf("primary administrator must retain implicit mutation access: %#v", decision)
	}
}

func TestAuthorizeCommentMutationRequiresReadScopeAndActionGrant(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "comment-mutation-primary", IsAdmin: true})
	withoutHidden := mustCreateUser(t, models.User{Username: "comment-mutation-without-hidden", IsAdmin: true})
	allowed := mustCreateUser(t, models.User{Username: "comment-mutation-allowed", IsAdmin: true})
	ordinary := mustCreateUser(t, models.User{Username: "comment-mutation-ordinary"})
	authorizer := authorization.New(db)
	if err := authorizer.ReplaceGrants(primary.ID, withoutHidden.ID, []authorization.Capability{authorization.CapabilityCommentsEdit}); err != nil {
		t.Fatalf("grant comment edit only: %v", err)
	}
	if err := authorizer.ReplaceGrants(primary.ID, allowed.ID, []authorization.Capability{authorization.CapabilityContentViewHidden, authorization.CapabilityCommentsEdit}); err != nil {
		t.Fatalf("grant hidden read and comment edit: %v", err)
	}
	message := models.Message{Content: "ordinary public", UserID: ordinary.ID, Visibility: MessageVisibilityPublic}
	ordinaryID := ordinary.ID
	privateComment := models.Comment{ID: 301, MessageID: 1, UserID: &ordinaryID, Visibility: MessageVisibilityPrivate}
	commentMap := CommentMap([]models.Comment{privateComment})

	if decision := AuthorizeCommentMutation(db, withoutHidden.ID, message, privateComment, commentMap, authorization.CapabilityCommentsEdit); decision.Allowed || decision.Reason != authorization.DenialContentNotReadable {
		t.Fatalf("comment edit without hidden read must be denied by read scope: %#v", decision)
	}
	if decision := AuthorizeCommentMutation(db, allowed.ID, message, privateComment, commentMap, authorization.CapabilityCommentsEdit); !decision.Allowed {
		t.Fatalf("hidden read plus comment edit must allow ordinary hidden comment: %#v", decision)
	}
}

func TestAuthorizeRecycleBinMutationRequiresViewPrerequisite(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "recycle-primary", IsAdmin: true})
	delegated := mustCreateUser(t, models.User{Username: "recycle-delegated", IsAdmin: true})
	ordinary := mustCreateUser(t, models.User{Username: "recycle-ordinary"})
	authorizer := authorization.New(db)
	if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityNotesRestore}); err != nil {
		t.Fatalf("grant restore only: %v", err)
	}
	if decision := AuthorizeRecycleBinMutation(db, delegated.ID, ordinary.ID, authorization.CapabilityNotesRestore); decision.Allowed || decision.Reason != authorization.DenialMissingPrerequisite {
		t.Fatalf("restore without recycle-bin view must be denied: %#v", decision)
	}
	if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityNotesRecycleBinView, authorization.CapabilityNotesRestore}); err != nil {
		t.Fatalf("grant recycle-bin view and restore: %v", err)
	}
	if decision := AuthorizeRecycleBinMutation(db, delegated.ID, ordinary.ID, authorization.CapabilityNotesRestore); !decision.Allowed {
		t.Fatalf("recycle-bin view plus restore must allow ordinary target: %#v", decision)
	}
}

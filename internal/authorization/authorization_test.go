package authorization

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAuthorizerTest(t *testing.T) (*gorm.DB, *Authorizer, models.User, models.User, models.User) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
		t.Fatalf("migrate authorization models: %v", err)
	}
	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true}
	delegated := models.User{Username: "delegated", IsAdmin: true}
	ordinary := models.User{Username: "ordinary"}
	for _, user := range []*models.User{&primary, &delegated, &ordinary} {
		if err := db.Create(user).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
	}
	return db, New(db), primary, delegated, ordinary
}

func TestAuthorizePrimaryAdminImplicitlyHasEveryCapability(t *testing.T) {
	_, authorizer, primary, _, _ := setupAuthorizerTest(t)

	decision := authorizer.Authorize(primary.ID, CapabilityAuthorizationManage, nil)
	if !decision.Allowed {
		t.Fatalf("primary administrator must be allowed: %#v", decision)
	}
}

func TestCatalogAuthorizationMatrixCoversEveryCapability(t *testing.T) {
	for _, definition := range Catalog() {
		definition := definition
		t.Run(string(definition.Capability), func(t *testing.T) {
			db, authorizer, primary, delegated, ordinary := setupAuthorizerTest(t)

			if decision := authorizer.Authorize(primary.ID, definition.Capability, nil); !decision.Allowed {
				t.Fatalf("primary administrator must be allowed: %#v", decision)
			}
			if decision := authorizer.Authorize(delegated.ID, definition.Capability, nil); decision.Allowed {
				t.Fatalf("delegated administrator without grant must be denied: %#v", decision)
			}
			if decision := authorizer.Authorize(ordinary.ID, definition.Capability, nil); decision.Allowed {
				t.Fatalf("ordinary user must be denied: %#v", decision)
			}

			if definition.Grantable {
				grants := []Capability{definition.Capability}
				for current := definition.Capability; ; {
					parent, ok := ParentCapabilityFor(current)
					if !ok {
						break
					}
					grants = append(grants, parent)
					current = parent
				}
				if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, grants); err != nil {
					t.Fatalf("grant %s: %v", definition.Capability, err)
				}
				if decision := authorizer.Authorize(delegated.ID, definition.Capability, nil); !decision.Allowed {
					t.Fatalf("delegated administrator with grant must be allowed: %#v", decision)
				}
				if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, nil); err != nil {
					t.Fatalf("revoke %s: %v", definition.Capability, err)
				}
				if decision := authorizer.Authorize(delegated.ID, definition.Capability, nil); decision.Allowed {
					t.Fatalf("revocation must apply immediately: %#v", decision)
				}
			} else {
				if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, []Capability{definition.Capability}); err == nil {
					t.Fatalf("primary-only capability must not be grantable: %s", definition.Capability)
				}
				if err := db.Create(&models.AdminCapabilityGrant{
					UserID: delegated.ID, Capability: string(definition.Capability), GrantedByUserID: primary.ID,
				}).Error; err != nil {
					t.Fatalf("create dirty grant fixture: %v", err)
				}
				if decision := authorizer.Authorize(delegated.ID, definition.Capability, nil); decision.Allowed {
					t.Fatalf("dirty primary-only grant must not authorize: %#v", decision)
				}
				capabilities, err := authorizer.CapabilitiesFor(delegated.ID)
				if err != nil {
					t.Fatalf("read delegated capabilities: %v", err)
				}
				for _, capability := range capabilities {
					if capability == definition.Capability {
						t.Fatalf("dirty primary-only grant must not appear in capability snapshot: %#v", capabilities)
					}
				}
			}

			if err := db.Create(&models.AdminCapabilityGrant{
				UserID: ordinary.ID, Capability: string(definition.Capability), GrantedByUserID: primary.ID,
			}).Error; err != nil {
				t.Fatalf("create ordinary-user dirty grant fixture: %v", err)
			}
			if decision := authorizer.Authorize(ordinary.ID, definition.Capability, nil); decision.Allowed {
				t.Fatalf("ordinary-user dirty grant must not authorize: %#v", decision)
			}
		})
	}
}

func TestAuthorizeDelegatedAdminRequiresGrantAndRevocationAppliesImmediately(t *testing.T) {
	_, authorizer, primary, delegated, _ := setupAuthorizerTest(t)

	if decision := authorizer.Authorize(delegated.ID, CapabilityAuditView, nil); decision.Allowed {
		t.Fatalf("delegated admin without grant must be denied: %#v", decision)
	}
	if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, []Capability{CapabilityAuditView}); err != nil {
		t.Fatalf("grant audit view: %v", err)
	}
	if decision := authorizer.Authorize(delegated.ID, CapabilityAuditView, nil); !decision.Allowed {
		t.Fatalf("delegated admin with grant must be allowed: %#v", decision)
	}
	if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, nil); err != nil {
		t.Fatalf("revoke audit view: %v", err)
	}
	if decision := authorizer.Authorize(delegated.ID, CapabilityAuditView, nil); decision.Allowed {
		t.Fatalf("revocation must apply on the next request: %#v", decision)
	}
}

func TestAuthorizeProtectsPrimaryAdminContentFromDelegatedMutation(t *testing.T) {
	_, authorizer, primary, delegated, _ := setupAuthorizerTest(t)
	if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, []Capability{CapabilityCommentsView, CapabilityCommentsTrash}); err != nil {
		t.Fatalf("grant comment deletion: %v", err)
	}

	ownerID := primary.ID
	decision := authorizer.Authorize(delegated.ID, CapabilityCommentsTrash, &ownerID)
	if decision.Allowed || decision.Reason != DenialProtectedContent {
		t.Fatalf("primary content must be protected: %#v", decision)
	}
}

func TestAuthorizeTargetOwnerMatrixPreservesProtectedContentBoundary(t *testing.T) {
	mutationCapabilities := []Capability{
		CapabilityCommentsEdit,
		CapabilityCommentsTrash,
		CapabilityNotesEdit,
		CapabilityNotesVisibility,
		CapabilityNotesPublishTime,
		CapabilityNotesPinGlobal,
		CapabilityNotesTrash,
		CapabilityNotesRestore,
		CapabilityNotesDelete,
	}
	for _, capability := range mutationCapabilities {
		capability := capability
		t.Run(string(capability), func(t *testing.T) {
			_, authorizer, primary, delegated, ordinary := setupAuthorizerTest(t)
			grants := []Capability{capability}
			if parent, ok := ParentCapabilityFor(capability); ok {
				grants = append(grants, parent)
			}
			if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, grants); err != nil {
				t.Fatalf("grant %s: %v", capability, err)
			}
			if decision := authorizer.Authorize(delegated.ID, capability, &ordinary.ID); !decision.Allowed {
				t.Fatalf("delegated administrator must manage ordinary target: %#v", decision)
			}
			if decision := authorizer.Authorize(delegated.ID, capability, &delegated.ID); !decision.Allowed {
				t.Fatalf("delegated administrator must manage own target: %#v", decision)
			}
			if decision := authorizer.Authorize(delegated.ID, capability, &primary.ID); decision.Allowed || decision.Reason != DenialProtectedContent {
				t.Fatalf("delegated administrator must not mutate primary-owned target: %#v", decision)
			}
			if decision := authorizer.Authorize(primary.ID, capability, &primary.ID); !decision.Allowed {
				t.Fatalf("primary administrator must retain protected-target access: %#v", decision)
			}
		})
	}

	for _, capability := range []Capability{CapabilityCommentsView, CapabilityNotesView} {
		capability := capability
		t.Run(string(capability)+"_does_not_block_read", func(t *testing.T) {
			_, authorizer, primary, delegated, _ := setupAuthorizerTest(t)
			if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, []Capability{capability}); err != nil {
				t.Fatalf("grant %s: %v", capability, err)
			}
			if decision := authorizer.Authorize(delegated.ID, capability, &primary.ID); !decision.Allowed {
				t.Fatalf("read capability must allow viewing primary-owned target: %#v", decision)
			}
		})
	}
}

func TestScopedHiddenContentAndRecycleBinViewCapabilitiesAreGrantableReads(t *testing.T) {
	for _, capability := range []Capability{"notes.view_hidden", "comments.view_hidden", CapabilityNotesRecycleBinView} {
		capability := capability
		t.Run(string(capability), func(t *testing.T) {
			_, authorizer, primary, delegated, _ := setupAuthorizerTest(t)
			definition, ok := DefinitionFor(capability)
			if !ok || !definition.Grantable {
				t.Fatalf("read capability must be present and grantable: %#v, %v", definition, ok)
			}
			grants := []Capability{capability}
			if parent, hasParent := ParentCapabilityFor(capability); hasParent {
				grants = append(grants, parent)
			}
			if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, grants); err != nil {
				t.Fatalf("grant %s: %v", capability, err)
			}
			if decision := authorizer.Authorize(delegated.ID, capability, &primary.ID); !decision.Allowed {
				t.Fatalf("read capability must not be classified as a protected mutation: %#v", decision)
			}
		})
	}
}

func TestEveryChildCapabilityHasTheExpectedParent(t *testing.T) {
	expected := map[Capability]Capability{
		CapabilityUsersResetPassword:         CapabilityUsersView,
		CapabilityUsersDelete:                CapabilityUsersView,
		CapabilityRegistrationReview:         CapabilityRegistrationView,
		"comments.view_hidden":               CapabilityCommentsView,
		CapabilityCommentsEdit:               CapabilityCommentsView,
		CapabilityCommentsTrash:              CapabilityCommentsView,
		CapabilityAttachmentsDownload:        CapabilityAttachmentsView,
		CapabilityAttachmentsDeleteReference: CapabilityAttachmentsView,
		CapabilityAttachmentsPurgeBlob:       CapabilityAttachmentsView,
		CapabilityStorageManage:              CapabilityStorageView,
		CapabilityDatabaseBackup:             CapabilityDatabaseView,
		CapabilityDatabaseRestore:            CapabilityDatabaseView,
		CapabilityVersionUpdate:              CapabilityVersionView,
		CapabilitySecurityManage:             CapabilitySecurityView,
		CapabilitySecurityClearLogs:          CapabilitySecurityView,
		CapabilityAccessLogsClear:            CapabilityAccessLogsView,
		CapabilitySiteVisitsClear:            CapabilitySiteVisitsView,
		CapabilitySiteSettingsManage:         CapabilitySiteSettingsView,
		CapabilityAnnouncementsManage:        CapabilityAnnouncementsView,
		CapabilityAnnouncementsPush:          CapabilityAnnouncementsView,
		CapabilityNotificationsManage:        CapabilityNotificationsView,
		CapabilityEmailManage:                CapabilityEmailView,
		"notes.view_hidden":                  CapabilityNotesView,
		CapabilityNotesEdit:                  CapabilityNotesView,
		CapabilityNotesVisibility:            CapabilityNotesView,
		CapabilityNotesPublishTime:           CapabilityNotesView,
		CapabilityNotesPinGlobal:             CapabilityNotesView,
		CapabilityNotesTrash:                 CapabilityNotesView,
		CapabilityNotesRestore:               CapabilityNotesRecycleBinView,
		CapabilityNotesDelete:                CapabilityNotesRecycleBinView,
	}

	for child, wantParent := range expected {
		gotParent, ok := ParentCapabilityFor(child)
		if !ok || gotParent != wantParent {
			t.Errorf("parent for %s = %s, %v; want %s", child, gotParent, ok, wantParent)
		}
	}
}

func TestReplaceGrantsRejectsPrimaryAndOrdinaryTargetsAndUnknownCapability(t *testing.T) {
	_, authorizer, primary, _, ordinary := setupAuthorizerTest(t)
	if err := authorizer.ReplaceGrants(primary.ID, primary.ID, []Capability{CapabilityAuditView}); err == nil {
		t.Fatal("primary administrator must not receive grant records")
	}
	if err := authorizer.ReplaceGrants(primary.ID, ordinary.ID, []Capability{CapabilityAuditView}); err == nil {
		t.Fatal("ordinary user must not receive grants")
	}
	if err := authorizer.ReplaceGrants(primary.ID, ordinary.ID, []Capability{"unknown.capability"}); err == nil {
		t.Fatal("unknown capability must be rejected")
	}
}

func TestReplaceGrantsRejectsChildCapabilityWithoutParent(t *testing.T) {
	_, authorizer, primary, delegated, _ := setupAuthorizerTest(t)

	err := authorizer.ReplaceGrants(primary.ID, delegated.ID, []Capability{CapabilityNotesEdit})
	if err == nil {
		t.Fatal("notes.edit without notes.view must be rejected")
	}
	if !strings.Contains(err.Error(), string(CapabilityNotesView)) {
		t.Fatalf("missing-parent error = %q, want parent capability", err)
	}
}

func TestAuthorizeAndCapabilitySnapshotRejectOrphanChildGrant(t *testing.T) {
	db, authorizer, primary, delegated, _ := setupAuthorizerTest(t)
	if err := db.Create(&models.AdminCapabilityGrant{
		UserID: delegated.ID, Capability: string(CapabilityNotesEdit), GrantedByUserID: primary.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}

	if decision := authorizer.Authorize(delegated.ID, CapabilityNotesEdit, nil); decision.Allowed || decision.Reason != DenialMissingPrerequisite {
		t.Fatalf("orphan child authorization = %#v, want missing prerequisite", decision)
	}
	capabilities, err := authorizer.CapabilitiesFor(delegated.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(capabilities) != 0 {
		t.Fatalf("orphan child leaked into capability snapshot: %#v", capabilities)
	}
}

func TestCatalogJSONExposesParentCapability(t *testing.T) {
	definition, ok := DefinitionFor(CapabilityNotesViewHidden)
	if !ok {
		t.Fatal("notes.view_hidden missing from catalog")
	}
	payload, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		ParentCapability Capability `json:"parent_capability"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ParentCapability != CapabilityNotesView {
		t.Fatalf("catalog parent = %s, want %s", decoded.ParentCapability, CapabilityNotesView)
	}
}

func TestReplaceGrantsIsIdempotentAndDoesNotAffectAnotherAdministrator(t *testing.T) {
	db, authorizer, primary, delegated, _ := setupAuthorizerTest(t)
	other := models.User{Username: "other-delegated", IsAdmin: true}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("create other delegated administrator: %v", err)
	}
	wanted := []Capability{CapabilityCommentsView, CapabilityAuditView, CapabilityCommentsView}
	if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, wanted); err != nil {
		t.Fatalf("replace delegated grants: %v", err)
	}
	if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, wanted); err != nil {
		t.Fatalf("repeat delegated grants: %v", err)
	}
	if err := authorizer.ReplaceGrants(primary.ID, other.ID, []Capability{CapabilityCommentsView}); err != nil {
		t.Fatalf("replace other delegated grants: %v", err)
	}

	got, err := authorizer.CapabilitiesFor(delegated.ID)
	if err != nil {
		t.Fatalf("read delegated grants: %v", err)
	}
	want := []Capability{CapabilityAuditView, CapabilityCommentsView}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("delegated capabilities=%v, want %v", got, want)
	}
	otherGot, err := authorizer.CapabilitiesFor(other.ID)
	if err != nil {
		t.Fatalf("read other delegated grants: %v", err)
	}
	if fmt.Sprint(otherGot) != fmt.Sprint([]Capability{CapabilityCommentsView}) {
		t.Fatalf("other delegated capabilities=%v", otherGot)
	}
	var count int64
	if err := db.Model(&models.AdminCapabilityGrant{}).Where("user_id = ?", delegated.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != int64(len(want)) {
		t.Fatalf("idempotent replace created %d grants, want %d", count, len(want))
	}
}

func TestAdminCapabilityGrantHasUniqueUserCapabilityKey(t *testing.T) {
	db, _, primary, delegated, _ := setupAuthorizerTest(t)
	grant := models.AdminCapabilityGrant{UserID: delegated.ID, Capability: string(CapabilityAuditView), GrantedByUserID: primary.ID}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatalf("create first grant: %v", err)
	}
	if err := db.Create(&models.AdminCapabilityGrant{UserID: delegated.ID, Capability: string(CapabilityAuditView), GrantedByUserID: primary.ID}).Error; err == nil {
		t.Fatal("duplicate user/capability grant must be rejected")
	}
}

func TestCatalogExcludesRetiredUserManageAndFeedRefreshCapabilities(t *testing.T) {
	for _, capability := range []Capability{"users.manage", "feed.manage", "rss.view", "rss.manage"} {
		if _, exists := DefinitionFor(capability); exists {
			t.Fatalf("retired capability %q must not appear in the authorization catalog", capability)
		}
	}
}

func TestCapabilitiesForOmitsRetiredGrants(t *testing.T) {
	db, authorizer, _, delegated, _ := setupAuthorizerTest(t)
	for _, capability := range []string{"users.manage", "feed.manage", "rss.view", "rss.manage"} {
		if err := db.Create(&models.AdminCapabilityGrant{UserID: delegated.ID, Capability: capability, GrantedByUserID: models.PrimaryAdminUserID}).Error; err != nil {
			t.Fatalf("create retired grant fixture %q: %v", capability, err)
		}
	}

	capabilities, err := authorizer.CapabilitiesFor(delegated.ID)
	if err != nil {
		t.Fatalf("load delegated capabilities: %v", err)
	}
	if len(capabilities) != 0 {
		t.Fatalf("retired grants must not be returned in the active capability snapshot: %#v", capabilities)
	}
}

func TestWriteAuditDeduplicatesIdenticalDeniedReadRequestsWithinWindow(t *testing.T) {
	db, authorizer, _, delegated, _ := setupAuthorizerTest(t)
	record := models.AdminAuditLog{
		ActorUserID: delegated.ID, Capability: string(CapabilitySecurityView), Module: "security",
		Action: "GET", TargetType: "route", TargetID: "/security", Result: "denied",
		Summary: "capability request denied", Reason: string(DenialMissingGrant), AuthVia: "session",
	}
	if err := authorizer.WriteAudit(record); err != nil {
		t.Fatalf("write first denied read audit: %v", err)
	}
	if err := authorizer.WriteAudit(record); err != nil {
		t.Fatalf("write duplicate denied read audit: %v", err)
	}
	var count int64
	if err := db.Model(&models.AdminAuditLog{}).Where("actor_user_id = ?", delegated.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("duplicate denied read should be suppressed, got %d records", count)
	}

	var existing models.AdminAuditLog
	if err := db.Where("actor_user_id = ?", delegated.ID).First(&existing).Error; err != nil {
		t.Fatalf("load denied read audit: %v", err)
	}
	if err := db.Model(&existing).Update("created_at", time.Now().Add(-auditDeniedReadDedupWindow-time.Second)).Error; err != nil {
		t.Fatalf("age denied read audit: %v", err)
	}
	if err := authorizer.WriteAudit(record); err != nil {
		t.Fatalf("write post-window denied read audit: %v", err)
	}
	if err := db.Model(&models.AdminAuditLog{}).Where("actor_user_id = ?", delegated.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("post-window denied read should be retained, got %d records", count)
	}
}

func TestWriteAuditDoesNotDeduplicateDifferentOrMutatingAuditRequests(t *testing.T) {
	db, authorizer, _, delegated, ordinary := setupAuthorizerTest(t)
	base := models.AdminAuditLog{
		ActorUserID: delegated.ID, Capability: string(CapabilitySecurityView), Module: "security",
		Action: "GET", TargetType: "route", Result: "denied",
		Summary: "capability request denied", Reason: string(DenialMissingGrant), AuthVia: "session",
	}
	cases := []struct {
		name   string
		mutate func(*models.AdminAuditLog, *models.AdminAuditLog)
	}{
		{name: "different capability", mutate: func(first, second *models.AdminAuditLog) {
			second.Capability = string(CapabilityNotificationsView)
			second.Module = "notifications"
		}},
		{name: "different route", mutate: func(first, second *models.AdminAuditLog) { second.TargetID = "/other-route" }},
		{name: "different reason", mutate: func(first, second *models.AdminAuditLog) { second.Reason = string(DenialProtectedContent) }},
		{name: "different actor", mutate: func(first, second *models.AdminAuditLog) { second.ActorUserID = ordinary.ID }},
		{name: "different auth source", mutate: func(first, second *models.AdminAuditLog) { second.AuthVia = "token" }},
		{name: "post mutation", mutate: func(first, second *models.AdminAuditLog) { first.Action = "POST"; second.Action = "POST" }},
		{name: "success result", mutate: func(first, second *models.AdminAuditLog) { first.Result = "success"; second.Result = "success" }},
		{name: "failure result", mutate: func(first, second *models.AdminAuditLog) { first.Result = "failure"; second.Result = "failure" }},
	}
	for index, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			first := base
			second := base
			first.TargetID = fmt.Sprintf("/audit-case-%d", index)
			second.TargetID = first.TargetID
			tt.mutate(&first, &second)
			if err := authorizer.WriteAudit(first); err != nil {
				t.Fatalf("write first audit: %v", err)
			}
			if err := authorizer.WriteAudit(second); err != nil {
				t.Fatalf("write second audit: %v", err)
			}
		})
	}
	var count int64
	if err := db.Model(&models.AdminAuditLog{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != int64(len(cases)*2) {
		t.Fatalf("different or mutating requests must remain auditable, got %d records", count)
	}
}

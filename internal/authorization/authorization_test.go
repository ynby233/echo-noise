package authorization

import (
	"testing"

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
	if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, []Capability{CapabilityCommentsDelete}); err != nil {
		t.Fatalf("grant comment deletion: %v", err)
	}

	ownerID := primary.ID
	decision := authorizer.Authorize(delegated.ID, CapabilityCommentsDelete, &ownerID)
	if decision.Allowed || decision.Reason != DenialProtectedContent {
		t.Fatalf("primary content must be protected: %#v", decision)
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

func TestCatalogExcludesRetiredUserManageAndFeedRefreshCapabilities(t *testing.T) {
	for _, capability := range []Capability{"users.manage", "feed.manage"} {
		if _, exists := DefinitionFor(capability); exists {
			t.Fatalf("retired capability %q must not appear in the authorization catalog", capability)
		}
	}
}

func TestCapabilitiesForOmitsRetiredGrants(t *testing.T) {
	db, authorizer, _, delegated, _ := setupAuthorizerTest(t)
	for _, capability := range []string{"users.manage", "feed.manage"} {
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

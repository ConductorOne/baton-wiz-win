package connector

import (
	"context"
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/stretchr/testify/require"
)

// buildTestUserResource creates a user resource with both role_id and
// project_ids populated in its profile, mirroring what userBuilder.List()
// produces in pkg/connector/users.go.
func buildTestUserResource(t *testing.T) *v2.Resource {
	t.Helper()

	profile := map[string]interface{}{
		"role_id":     "role-123",
		"project_ids": []interface{}{"project-abc", "project-def"},
	}

	userRes, err := resource.NewUserResource(
		"test@example.com",
		userResourceType,
		"test@example.com",
		[]resource.UserTraitOption{
			resource.WithEmail("test@example.com", true),
		},
		resource.WithResourceProfile(profile),
	)
	require.NoError(t, err)

	return userRes
}

// grantsForResourceType filters grants to those whose entitlement resource
// is of the given resource type ID (e.g. RoleResourceTypeID, ProjectResourceTypeID).
func grantsForResourceType(grants []*v2.Grant, resourceTypeID string) []*v2.Grant {
	var out []*v2.Grant
	for _, g := range grants {
		if g.GetEntitlement().GetResource().GetId().GetResourceType() == resourceTypeID {
			out = append(out, g)
		}
	}
	return out
}

func TestUserBuilder_Grants_GatedBySyncFilter(t *testing.T) {
	ctx := context.Background()

	t.Run("both flags true emits both role and project grants", func(t *testing.T) {
		u := newUserBuilder(nil, true, true)
		userRes := buildTestUserResource(t)

		grants, _, err := u.Grants(ctx, userRes, resource.SyncOpAttrs{})
		require.NoError(t, err)

		require.Len(t, grantsForResourceType(grants, RoleResourceTypeID), 1, "expected exactly one role grant")
		require.Len(t, grantsForResourceType(grants, ProjectResourceTypeID), 2, "expected two project grants")
		require.Len(t, grants, 3)
	})

	t.Run("syncRoles false suppresses role grant but keeps project grants", func(t *testing.T) {
		u := newUserBuilder(nil, false, true)
		userRes := buildTestUserResource(t)

		grants, _, err := u.Grants(ctx, userRes, resource.SyncOpAttrs{})
		require.NoError(t, err)

		require.Empty(t, grantsForResourceType(grants, RoleResourceTypeID), "role grant should be suppressed")
		require.Len(t, grantsForResourceType(grants, ProjectResourceTypeID), 2, "project grants should still be emitted")
	})

	t.Run("syncProjects false suppresses project grants but keeps role grant", func(t *testing.T) {
		u := newUserBuilder(nil, true, false)
		userRes := buildTestUserResource(t)

		grants, _, err := u.Grants(ctx, userRes, resource.SyncOpAttrs{})
		require.NoError(t, err)

		require.Len(t, grantsForResourceType(grants, RoleResourceTypeID), 1, "role grant should still be emitted")
		require.Empty(t, grantsForResourceType(grants, ProjectResourceTypeID), "project grants should be suppressed")
	})

	t.Run("both flags false suppresses all cross-type grants", func(t *testing.T) {
		u := newUserBuilder(nil, false, false)
		userRes := buildTestUserResource(t)

		grants, _, err := u.Grants(ctx, userRes, resource.SyncOpAttrs{})
		require.NoError(t, err)
		require.Empty(t, grants)
	})
}

// TestUserBuilder_ResourceType_Annotations asserts the resource-type-level
// annotation escalation described in userBuilder's syncRoles/syncProjects
// doc comment: SkipEntitlementsAndGrants is only added when NEITHER cross-type
// target would be emitted. Whenever at least one of role/project is still
// being synced, Grants() must still run (gated internally per-target), so the
// annotation must not escalate in that case.
func TestUserBuilder_ResourceType_Annotations(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name                          string
		syncRoles, syncProjects       bool
		wantSkipEntitlementsAndGrants bool
	}{
		{"both synced", true, true, false},
		{"only roles synced", true, false, false},
		{"only projects synced", false, true, false},
		{"neither synced", false, false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := newUserBuilder(nil, tc.syncRoles, tc.syncProjects)
			rt := u.ResourceType(ctx)

			annos := annotations.Annotations(rt.GetAnnotations())
			require.True(t, annos.Contains(&v2.SkipEntitlements{}), "base SkipEntitlements annotation should always be present")
			require.Equal(t, tc.wantSkipEntitlementsAndGrants, annos.Contains(&v2.SkipEntitlementsAndGrants{}))
		})
	}

	t.Run("does not mutate the shared package-level resource type", func(t *testing.T) {
		before := annotations.Annotations(userResourceType.GetAnnotations())
		require.False(t, before.Contains(&v2.SkipEntitlementsAndGrants{}))

		u := newUserBuilder(nil, false, false)
		rt := u.ResourceType(ctx)
		rtAnnos := annotations.Annotations(rt.GetAnnotations())
		require.True(t, rtAnnos.Contains(&v2.SkipEntitlementsAndGrants{}))

		after := annotations.Annotations(userResourceType.GetAnnotations())
		require.False(t, after.Contains(&v2.SkipEntitlementsAndGrants{}), "package-level userResourceType must remain unmodified")
	})
}

package connector

import (
	"context"
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
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
			resource.WithUserProfile(profile),
		},
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

package connector

import (
	"context"
	"strings"
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-wiz-win/pkg/wiz"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type userRoleUpdate struct{ userID, roleID string }

type fakeClient struct {
	users        map[string]*wiz.User
	projects     map[string]*wiz.Project
	listUsers    *wiz.UserConnection
	listProjects *wiz.ProjectConnection

	emailLookups   int
	roleUpdates    []userRoleUpdate
	projectUpdates map[string][]string
	memberUpdates  map[string][]string
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		users:          map[string]*wiz.User{},
		projects:       map[string]*wiz.Project{},
		projectUpdates: map[string][]string{},
		memberUpdates:  map[string][]string{},
	}
}

func (f *fakeClient) ListUsers(context.Context, *string) (*wiz.UserConnection, error) {
	return f.listUsers, nil
}

func (f *fakeClient) ListProjects(context.Context, *string) (*wiz.ProjectConnection, error) {
	return f.listProjects, nil
}

func (f *fakeClient) ListUserRoles(context.Context, *string) (*wiz.UserRoleConnection, error) {
	return &wiz.UserRoleConnection{}, nil
}

func (f *fakeClient) GetUser(_ context.Context, id string) (*wiz.User, error) {
	return f.users[id], nil
}

func (f *fakeClient) FindUserByEmail(_ context.Context, email string) (*wiz.User, error) {
	f.emailLookups++
	for _, u := range f.users {
		if strings.EqualFold(u.Email, email) {
			return u, nil
		}
	}
	return nil, nil
}

func (f *fakeClient) UpdateUserRole(_ context.Context, userID, roleID string) error {
	f.roleUpdates = append(f.roleUpdates, userRoleUpdate{userID, roleID})
	f.users[userID].EffectiveRole.ID = roleID
	return nil
}

func (f *fakeClient) UpdateUserProjects(_ context.Context, userID string, ids []string) error {
	f.projectUpdates[userID] = ids
	refs := make([]wiz.ProjectRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, wiz.ProjectRef{ID: id})
	}
	f.users[userID].EffectiveAssignedProjects = refs
	return nil
}

func (f *fakeClient) GetProject(_ context.Context, id string) (*wiz.Project, error) {
	return f.projects[id], nil
}

func (f *fakeClient) UpdateProjectMembers(_ context.Context, projectID string, field wiz.ProjectMemberField, ids []string) error {
	f.memberUpdates[projectID+"/"+string(field)] = ids
	return nil
}

func (f *fakeClient) LockObject(string) func() { return func() {} }

// testPrincipal builds a user principal as synced by userBuilder.List: the
// resource ID is the email and the external ID is the Wiz user ID.
func testPrincipal(t *testing.T, email, wizID string) *v2.Resource {
	t.Helper()
	opts := []resource.ResourceOption{}
	if wizID != "" {
		opts = append(opts, resource.WithExternalID(&v2.ExternalId{Id: wizID}))
	}
	r, err := resource.NewUserResource(email, userResourceType, email,
		[]resource.UserTraitOption{resource.WithEmail(email, true)}, opts...)
	require.NoError(t, err)
	return r
}

// testProfilePrincipal records the Wiz user ID only in the profile, as C1
// may not return the deprecated external ID.
func testProfilePrincipal(t *testing.T, email, wizID string) *v2.Resource {
	t.Helper()
	r, err := resource.NewUserResource(email, userResourceType, email,
		[]resource.UserTraitOption{resource.WithEmail(email, true)},
		resource.WithResourceProfile(map[string]interface{}{profileUserIDKey: wizID}))
	require.NoError(t, err)
	return r
}

func testEntitlement(t *testing.T, rt *v2.ResourceType, resourceID, slug string) *v2.Entitlement {
	t.Helper()
	r, err := resource.NewResource(resourceID, rt, resourceID)
	require.NoError(t, err)
	return &v2.Entitlement{Id: rt.Id + ":" + resourceID + ":" + slug, Resource: r, Slug: slug}
}


func revokeGrant(principal *v2.Resource, e *v2.Entitlement) *v2.Grant {
	return &v2.Grant{Entitlement: e, Principal: principal}
}


func TestUserListSetsWizUserExternalID(t *testing.T) {
	f := newFakeClient()
	f.listUsers = &wiz.UserConnection{Nodes: []wiz.User{{ID: "u1", Email: "a@example.com"}}}
	res, results, err := newUserBuilder(f, true, true).List(context.Background(), nil, resource.SyncOpAttrs{})
	require.NoError(t, err)
	require.Empty(t, results.NextPageToken)
	require.Len(t, res, 1)
	require.Equal(t, "a@example.com", res[0].GetId().GetResource(), "the user resource ID stays the email")
	require.Equal(t, "u1", res[0].GetExternalId().GetId()) //nolint:staticcheck // Asserting the deprecated field is still set.
	require.Equal(t, []string{"u1"}, profileStrings(res[0], profileUserIDKey))
}

func TestListFailsWhenMorePagesHaveNoCursor(t *testing.T) {
	f := newFakeClient()
	f.listUsers = &wiz.UserConnection{PageInfo: wiz.PageInfo{HasNextPage: true}}
	f.listProjects = &wiz.ProjectConnection{PageInfo: wiz.PageInfo{HasNextPage: true}}

	_, _, err := newUserBuilder(f, true, true).List(context.Background(), nil, resource.SyncOpAttrs{})
	require.Error(t, err)
	_, _, err = newProjectBuilder(f).List(context.Background(), nil, resource.SyncOpAttrs{})
	require.Error(t, err)
}

func TestListPassesCursor(t *testing.T) {
	f := newFakeClient()
	f.listProjects = &wiz.ProjectConnection{PageInfo: wiz.PageInfo{HasNextPage: true, EndCursor: "c1"}}
	_, results, err := newProjectBuilder(f).List(context.Background(), nil, resource.SyncOpAttrs{PageToken: pagination.Token{}})
	require.NoError(t, err)
	require.Equal(t, "c1", results.NextPageToken)
}

func TestProjectSyncEmitsOwnerAndChampionGrants(t *testing.T) {
	f := newFakeClient()
	f.listProjects = &wiz.ProjectConnection{Nodes: []wiz.Project{{
		ID:                "p1",
		Name:              "Prod",
		ProjectOwners:     []wiz.ProjectOwner{{ID: "u1", Email: "a@example.com"}, {ID: "u3"}},
		SecurityChampions: []wiz.SecurityChampion{{ID: "u2", Email: "b@example.com"}},
	}}}
	p := newProjectBuilder(f)
	res, _, err := p.List(context.Background(), nil, resource.SyncOpAttrs{})
	require.NoError(t, err)
	require.Len(t, res, 1)

	grants, results, err := p.Grants(context.Background(), res[0], resource.SyncOpAttrs{})
	require.NoError(t, err)
	require.Nil(t, results)
	got := map[string]string{}
	for _, g := range grants {
		got[g.GetPrincipal().GetId().GetResource()] = entitlementSlug(g.GetEntitlement())
		require.Equal(t, UserResourceTypeID, g.GetPrincipal().GetId().GetResourceType())
	}
	require.Equal(t, map[string]string{"a@example.com": ownerEntitlement, "b@example.com": championEntitlement}, got,
		"grants keep the email principal IDs; members without an email are skipped as before")
}

func TestRoleGrantAndRevoke(t *testing.T) {
	ctx := context.Background()
	f := newFakeClient()
	f.users["u1"] = &wiz.User{ID: "u1", Email: "a@example.com", EffectiveRole: wiz.UserRoleRef{ID: "GLOBAL_READER"}}
	r := newRoleBuilder(f)
	principal := testPrincipal(t, "a@example.com", "u1")
	admin := testEntitlement(t, roleResourceType, "GLOBAL_ADMIN", memberEntitlement)

	grants, annos, err := r.Grant(ctx, principal, admin)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.Empty(t, annos)
	require.Equal(t, []userRoleUpdate{{"u1", "GLOBAL_ADMIN"}}, f.roleUpdates)

	_, annos, err = r.Grant(ctx, principal, admin)
	require.NoError(t, err)
	require.True(t, annos.Contains(&v2.GrantAlreadyExists{}))
	require.Len(t, f.roleUpdates, 1, "an existing grant must not be re-applied")

	// Revoking the held role is refused: every Wiz user must hold a role.
	_, err = r.Revoke(ctx, revokeGrant(principal, admin))
	require.Equal(t, codes.FailedPrecondition, status.Code(err))

	// Revoking a role the user no longer holds is already revoked.
	reader := testEntitlement(t, roleResourceType, "GLOBAL_READER", memberEntitlement)
	annos, err = r.Revoke(ctx, revokeGrant(principal, reader))
	require.NoError(t, err)
	require.True(t, annos.Contains(&v2.GrantAlreadyRevoked{}))
}

func TestProvisioningMissingUser(t *testing.T) {
	ctx := context.Background()
	f := newFakeClient()
	principal := testPrincipal(t, "gone@example.com", "gone")

	_, _, err := newRoleBuilder(f).Grant(ctx, principal, testEntitlement(t, roleResourceType, "GLOBAL_ADMIN", memberEntitlement))
	require.Equal(t, codes.NotFound, status.Code(err))

	annos, err := newProjectBuilder(f).Revoke(ctx, revokeGrant(principal, testEntitlement(t, projectResourceType, "p1", memberEntitlement)))
	require.NoError(t, err)
	require.True(t, annos.Contains(&v2.GrantAlreadyRevoked{}))
}

func TestProvisioningResolvesWizUserID(t *testing.T) {
	ctx := context.Background()
	admin := testEntitlement(t, roleResourceType, "GLOBAL_ADMIN", memberEntitlement)

	t.Run("profile user_id without external ID", func(t *testing.T) {
		f := newFakeClient()
		// The email differs from the resource ID, so only an ID lookup can find the user.
		f.users["u1"] = &wiz.User{ID: "u1", Email: "renamed@example.com"}
		_, _, err := newRoleBuilder(f).Grant(ctx, testProfilePrincipal(t, "a@example.com", "u1"), admin)
		require.NoError(t, err)
		require.Equal(t, []userRoleUpdate{{"u1", "GLOBAL_ADMIN"}}, f.roleUpdates)
		require.Zero(t, f.emailLookups, "a recorded Wiz user ID must not trigger an email search")
	})

	t.Run("email fallback for principals synced before the ID was recorded", func(t *testing.T) {
		f := newFakeClient()
		f.users["u1"] = &wiz.User{ID: "u1", Email: "A@example.com"}
		_, _, err := newRoleBuilder(f).Grant(ctx, testPrincipal(t, "a@example.com", ""), admin)
		require.NoError(t, err)
		require.Equal(t, []userRoleUpdate{{"u1", "GLOBAL_ADMIN"}}, f.roleUpdates)
		require.Equal(t, 1, f.emailLookups)
	})
}

func TestProvisioningRejectsNonUserPrincipal(t *testing.T) {
	f := newFakeClient()
	principal, err := resource.NewResource("p2", projectResourceType, "p2")
	require.NoError(t, err)
	_, _, err = newProjectBuilder(f).Grant(context.Background(), principal, testEntitlement(t, projectResourceType, "p1", ownerEntitlement))
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestProjectMemberGrantAndRevoke(t *testing.T) {
	ctx := context.Background()
	f := newFakeClient()
	f.users["u1"] = &wiz.User{
		ID:                        "u1",
		Email:                     "a@example.com",
		EffectiveRole:             wiz.UserRoleRef{ID: "PROJECT_READER", IsProjectScoped: true},
		EffectiveAssignedProjects: []wiz.ProjectRef{{ID: "p0"}},
	}
	p := newProjectBuilder(f)
	principal := testPrincipal(t, "a@example.com", "u1")
	e := testEntitlement(t, projectResourceType, "p1", memberEntitlement)

	_, annos, err := p.Grant(ctx, principal, e)
	require.NoError(t, err)
	require.Empty(t, annos)
	require.Equal(t, []string{"p0", "p1"}, f.projectUpdates["u1"])

	_, annos, err = p.Grant(ctx, principal, e)
	require.NoError(t, err)
	require.True(t, annos.Contains(&v2.GrantAlreadyExists{}))

	annos, err = p.Revoke(ctx, revokeGrant(principal, e))
	require.NoError(t, err)
	require.Empty(t, annos)
	require.Equal(t, []string{"p0"}, f.projectUpdates["u1"])

	annos, err = p.Revoke(ctx, revokeGrant(principal, e))
	require.NoError(t, err)
	require.True(t, annos.Contains(&v2.GrantAlreadyRevoked{}))
}

func TestProjectMemberRevokeLastProjectSendsEmptyList(t *testing.T) {
	f := newFakeClient()
	f.users["u1"] = &wiz.User{
		ID:                        "u1",
		EffectiveRole:             wiz.UserRoleRef{ID: "PROJECT_READER", IsProjectScoped: true},
		EffectiveAssignedProjects: []wiz.ProjectRef{{ID: "p1"}},
	}
	_, err := newProjectBuilder(f).Revoke(context.Background(),
		revokeGrant(testPrincipal(t, "a@example.com", "u1"), testEntitlement(t, projectResourceType, "p1", memberEntitlement)))
	require.NoError(t, err)
	ids, ok := f.projectUpdates["u1"]
	require.True(t, ok)
	require.NotNil(t, ids)
	require.Empty(t, ids)
}

func TestProjectMemberGrantRejectsGlobalRole(t *testing.T) {
	f := newFakeClient()
	f.users["u1"] = &wiz.User{ID: "u1", EffectiveRole: wiz.UserRoleRef{ID: "GLOBAL_READER"}}
	_, _, err := newProjectBuilder(f).Grant(context.Background(),
		testPrincipal(t, "a@example.com", "u1"), testEntitlement(t, projectResourceType, "p1", memberEntitlement))
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.Empty(t, f.projectUpdates)
}

func TestProjectOwnerAndChampionGrantAndRevoke(t *testing.T) {
	for _, tc := range []struct {
		slug  string
		field wiz.ProjectMemberField
	}{
		{ownerEntitlement, wiz.ProjectOwnersField},
		{championEntitlement, wiz.SecurityChampionsField},
	} {
		t.Run(tc.slug, func(t *testing.T) {
			ctx := context.Background()
			f := newFakeClient()
			f.users["u1"] = &wiz.User{ID: "u1", Email: "a@example.com"}
			f.projects["p1"] = &wiz.Project{
				ID:                "p1",
				ProjectOwners:     []wiz.ProjectOwner{{ID: "u9", Email: "z@example.com"}},
				SecurityChampions: []wiz.SecurityChampion{{ID: "u9", Email: "z@example.com"}},
			}
			p := newProjectBuilder(f)
			principal := testPrincipal(t, "a@example.com", "u1")
			e := testEntitlement(t, projectResourceType, "p1", tc.slug)
			key := "p1/" + string(tc.field)

			_, annos, err := p.Grant(ctx, principal, e)
			require.NoError(t, err)
			require.Empty(t, annos)
			require.Equal(t, []string{"u9", "u1"}, f.memberUpdates[key])

			// Already listed (matched by email, case-insensitive).
			if tc.field == wiz.ProjectOwnersField {
				f.projects["p1"].ProjectOwners = append(f.projects["p1"].ProjectOwners, wiz.ProjectOwner{ID: "u1", Email: "A@example.com"})
			} else {
				f.projects["p1"].SecurityChampions = append(f.projects["p1"].SecurityChampions, wiz.SecurityChampion{ID: "u1", Email: "A@example.com"})
			}
			_, annos, err = p.Grant(ctx, principal, e)
			require.NoError(t, err)
			require.True(t, annos.Contains(&v2.GrantAlreadyExists{}))

			annos, err = p.Revoke(ctx, revokeGrant(principal, e))
			require.NoError(t, err)
			require.Empty(t, annos)
			require.Equal(t, []string{"u9"}, f.memberUpdates[key])
		})
	}
}

func TestProjectOwnerGrantRejectsMemberWithoutID(t *testing.T) {
	f := newFakeClient()
	f.users["u1"] = &wiz.User{ID: "u1", Email: "a@example.com"}
	f.projects["p1"] = &wiz.Project{ID: "p1", ProjectOwners: []wiz.ProjectOwner{{Email: "noid@example.com"}}}
	_, _, err := newProjectBuilder(f).Grant(context.Background(),
		testPrincipal(t, "a@example.com", "u1"), testEntitlement(t, projectResourceType, "p1", ownerEntitlement))
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.Empty(t, f.memberUpdates)
}

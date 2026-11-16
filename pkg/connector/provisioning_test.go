package connector

import (
	"context"
	"sync"
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-wiz-win/pkg/wiz"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeWizClient struct {
	users    map[string]*wiz.UserDetail
	projects map[string]*wiz.ProjectDetail

	roleUpdates    []string
	projectUpdates [][]string
	memberUpdates  map[wiz.ProjectMemberField][]string

	mu sync.Mutex
}

func newFakeWizClient() *fakeWizClient {
	return &fakeWizClient{
		users:         map[string]*wiz.UserDetail{},
		projects:      map[string]*wiz.ProjectDetail{},
		memberUpdates: map[wiz.ProjectMemberField][]string{},
	}
}

func (f *fakeWizClient) ListUsers(context.Context, *string) (*wiz.UserConnection, error) {
	return &wiz.UserConnection{}, nil
}

func (f *fakeWizClient) ListProjects(context.Context, *string) (*wiz.ProjectConnection, error) {
	return &wiz.ProjectConnection{}, nil
}

func (f *fakeWizClient) ListUserRoles(context.Context, *string) (*wiz.UserRoleConnection, error) {
	return &wiz.UserRoleConnection{}, nil
}

func (f *fakeWizClient) GetUser(_ context.Context, id string) (*wiz.UserDetail, error) {
	return f.users[id], nil
}

func (f *fakeWizClient) FindUserByEmail(_ context.Context, email string) (*wiz.UserDetail, error) {
	for _, u := range f.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, nil
}

func (f *fakeWizClient) UpdateUserRole(_ context.Context, userID, roleID string) error {
	f.roleUpdates = append(f.roleUpdates, roleID)
	f.users[userID].EffectiveRole.ID = roleID
	return nil
}

func (f *fakeWizClient) UpdateUserProjects(_ context.Context, userID string, ids []string) error {
	f.projectUpdates = append(f.projectUpdates, ids)
	refs := make([]wiz.ProjectRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, wiz.ProjectRef{ID: id})
	}
	f.users[userID].EffectiveAssignedProjects = refs
	return nil
}

func (f *fakeWizClient) GetProject(_ context.Context, id string) (*wiz.ProjectDetail, error) {
	return f.projects[id], nil
}

func (f *fakeWizClient) UpdateProjectMembers(_ context.Context, projectID string, field wiz.ProjectMemberField, ids []string) error {
	f.memberUpdates[field] = ids
	members := make([]wiz.ProjectMember, 0, len(ids))
	for _, id := range ids {
		members = append(members, wiz.ProjectMember{ID: id})
	}
	if field == wiz.ProjectOwnersField {
		f.projects[projectID].ProjectOwners = members
	} else {
		f.projects[projectID].SecurityChampions = members
	}
	return nil
}

func (f *fakeWizClient) LockObject(string) func() {
	f.mu.Lock()
	return f.mu.Unlock
}

func testPrincipal(t *testing.T, email, wizID string) *v2.Resource {
	t.Helper()
	profile := map[string]interface{}{}
	if wizID != "" {
		profile[profileUserIDKey] = wizID
	}
	r, err := resource.NewUserResource(email, userResourceType, email,
		[]resource.UserTraitOption{resource.WithEmail(email, true)},
		resource.WithResourceProfile(profile))
	require.NoError(t, err)
	return r
}

func testEntitlement(t *testing.T, rt *v2.ResourceType, resourceID, slug string) *v2.Entitlement {
	t.Helper()
	r, err := resource.NewResource(resourceID, rt, resourceID)
	require.NoError(t, err)
	return &v2.Entitlement{Id: rt.GetId() + ":" + resourceID + ":" + slug, Resource: r, Slug: slug}
}

func TestRoleGrantAndRevoke(t *testing.T) {
	ctx := context.Background()
	f := newFakeWizClient()
	f.users["u1"] = &wiz.UserDetail{ID: "u1", Email: "a@example.com", EffectiveRole: wiz.UserRoleDetail{ID: "GLOBAL_READER"}}
	r := newRoleBuilder(f)
	principal := testPrincipal(t, "a@example.com", "u1")
	adminEnt := testEntitlement(t, roleResourceType, "GLOBAL_ADMIN", memberEntitlement)

	grants, annos, err := r.Grant(ctx, principal, adminEnt)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.Empty(t, annos)
	require.Equal(t, []string{"GLOBAL_ADMIN"}, f.roleUpdates)
	require.Equal(t, "GLOBAL_ADMIN", grants[0].GetEntitlement().GetResource().GetId().GetResource())

	// Granting the held role is idempotent.
	_, annos, err = r.Grant(ctx, principal, adminEnt)
	require.NoError(t, err)
	require.True(t, annos.Contains(&v2.GrantAlreadyExists{}))
	require.Len(t, f.roleUpdates, 1)

	// Revoking a role the user no longer holds succeeds as already revoked.
	readerGrant := &v2.Grant{Principal: principal, Entitlement: testEntitlement(t, roleResourceType, "GLOBAL_READER", memberEntitlement)}
	annos, err = r.Revoke(ctx, readerGrant)
	require.NoError(t, err)
	require.True(t, annos.Contains(&v2.GrantAlreadyRevoked{}))

	// Revoking the held role is refused because every user needs a role.
	_, err = r.Revoke(ctx, &v2.Grant{Principal: principal, Entitlement: adminEnt})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestRoleGrantFallsBackToEmailLookup(t *testing.T) {
	ctx := context.Background()
	f := newFakeWizClient()
	f.users["u1"] = &wiz.UserDetail{ID: "u1", Email: "a@example.com", EffectiveRole: wiz.UserRoleDetail{ID: "GLOBAL_READER"}}
	r := newRoleBuilder(f)

	_, _, err := r.Grant(ctx, testPrincipal(t, "a@example.com", ""), testEntitlement(t, roleResourceType, "GLOBAL_ADMIN", memberEntitlement))
	require.NoError(t, err)
	require.Equal(t, "GLOBAL_ADMIN", f.users["u1"].EffectiveRole.ID)

	_, _, err = r.Grant(ctx, testPrincipal(t, "missing@example.com", ""), testEntitlement(t, roleResourceType, "GLOBAL_ADMIN", memberEntitlement))
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestProjectMemberGrantAndRevoke(t *testing.T) {
	ctx := context.Background()
	f := newFakeWizClient()
	f.users["u1"] = &wiz.UserDetail{
		ID:                        "u1",
		Email:                     "a@example.com",
		EffectiveRole:             wiz.UserRoleDetail{ID: "PROJECT_READER", IsProjectScoped: true},
		EffectiveAssignedProjects: []wiz.ProjectRef{{ID: "p0"}},
	}
	p := newProjectBuilder(f)
	principal := testPrincipal(t, "a@example.com", "u1")
	ent := testEntitlement(t, projectResourceType, "p1", memberEntitlement)

	_, annos, err := p.Grant(ctx, principal, ent)
	require.NoError(t, err)
	require.Empty(t, annos)
	require.Equal(t, []string{"p0", "p1"}, f.projectUpdates[0], "existing projects are preserved")

	_, annos, err = p.Grant(ctx, principal, ent)
	require.NoError(t, err)
	require.True(t, annos.Contains(&v2.GrantAlreadyExists{}))

	annos, err = p.Revoke(ctx, &v2.Grant{Principal: principal, Entitlement: ent})
	require.NoError(t, err)
	require.Empty(t, annos)
	require.Equal(t, []string{"p0"}, f.projectUpdates[1])

	annos, err = p.Revoke(ctx, &v2.Grant{Principal: principal, Entitlement: ent})
	require.NoError(t, err)
	require.True(t, annos.Contains(&v2.GrantAlreadyRevoked{}))
	require.Len(t, f.projectUpdates, 2)
}

func TestProjectMemberRevokeLastProjectSendsEmptyList(t *testing.T) {
	ctx := context.Background()
	f := newFakeWizClient()
	f.users["u1"] = &wiz.UserDetail{
		ID:                        "u1",
		Email:                     "a@example.com",
		EffectiveRole:             wiz.UserRoleDetail{ID: "PROJECT_READER", IsProjectScoped: true},
		EffectiveAssignedProjects: []wiz.ProjectRef{{ID: "p1"}},
	}
	p := newProjectBuilder(f)
	_, err := p.Revoke(ctx, &v2.Grant{
		Principal:   testPrincipal(t, "a@example.com", "u1"),
		Entitlement: testEntitlement(t, projectResourceType, "p1", memberEntitlement),
	})
	require.NoError(t, err)
	require.NotNil(t, f.projectUpdates[0])
	require.Empty(t, f.projectUpdates[0])
}

func TestProjectMemberGrantRejectsGlobalRole(t *testing.T) {
	ctx := context.Background()
	f := newFakeWizClient()
	f.users["u1"] = &wiz.UserDetail{ID: "u1", Email: "a@example.com", EffectiveRole: wiz.UserRoleDetail{ID: "GLOBAL_READER"}}
	p := newProjectBuilder(f)

	_, _, err := p.Grant(ctx, testPrincipal(t, "a@example.com", "u1"), testEntitlement(t, projectResourceType, "p1", memberEntitlement))
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.Empty(t, f.projectUpdates)
}

func TestProjectOwnerAndChampionGrantAndRevoke(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		slug  string
		field wiz.ProjectMemberField
	}{
		{ownerEntitlement, wiz.ProjectOwnersField},
		{championEntitlement, wiz.SecurityChampionsField},
	} {
		t.Run(tc.slug, func(t *testing.T) {
			f := newFakeWizClient()
			f.users["u1"] = &wiz.UserDetail{ID: "u1", Email: "a@example.com"}
			f.projects["p1"] = &wiz.ProjectDetail{
				ID:                "p1",
				ProjectOwners:     []wiz.ProjectMember{{ID: "u9", Email: "other@example.com"}},
				SecurityChampions: []wiz.ProjectMember{{ID: "u9", Email: "other@example.com"}},
			}
			p := newProjectBuilder(f)
			principal := testPrincipal(t, "a@example.com", "u1")
			ent := testEntitlement(t, projectResourceType, "p1", tc.slug)

			grants, annos, err := p.Grant(ctx, principal, ent)
			require.NoError(t, err)
			require.Empty(t, annos)
			require.Equal(t, "project:p1:"+tc.slug, grants[0].GetEntitlement().GetId())
			require.Equal(t, []string{"u9", "u1"}, f.memberUpdates[tc.field])

			_, annos, err = p.Grant(ctx, principal, ent)
			require.NoError(t, err)
			require.True(t, annos.Contains(&v2.GrantAlreadyExists{}))

			annos, err = p.Revoke(ctx, &v2.Grant{Principal: principal, Entitlement: ent})
			require.NoError(t, err)
			require.Empty(t, annos)
			require.Equal(t, []string{"u9"}, f.memberUpdates[tc.field])

			annos, err = p.Revoke(ctx, &v2.Grant{Principal: principal, Entitlement: ent})
			require.NoError(t, err)
			require.True(t, annos.Contains(&v2.GrantAlreadyRevoked{}))
		})
	}
}

func TestProvisioningRejectsNonUserPrincipal(t *testing.T) {
	ctx := context.Background()
	p := newProjectBuilder(newFakeWizClient())
	principal, err := resource.NewResource("p2", projectResourceType, "p2")
	require.NoError(t, err)

	_, _, err = p.Grant(ctx, principal, testEntitlement(t, projectResourceType, "p1", ownerEntitlement))
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

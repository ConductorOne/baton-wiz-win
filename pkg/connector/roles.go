package connector

import (
	"context"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	"github.com/conductorone/baton-sdk/pkg/types/grant"
	"github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-wiz-win/pkg/wiz"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type roleBuilder struct {
	client wiz.Client
}

func (r *roleBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return roleResourceType
}

// List returns roles from Wiz as resource objects, one page at a time.
func (r *roleBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, attr resource.SyncOpAttrs) ([]*v2.Resource, *resource.SyncOpResults, error) {
	var resources []*v2.Resource

	// Get the page token from the sync attributes
	var cursor *string
	if attr.PageToken.Token != "" {
		cursor = &attr.PageToken.Token
	}

	// Fetch one page of roles
	resp, err := r.client.ListUserRoles(ctx, cursor)
	if err != nil {
		return nil, nil, fmt.Errorf("wiz-connector: failed to list roles: %w", err)
	}

	for _, role := range resp.Nodes {
		roleResource, err := resource.NewRoleResource(
			role.Name,
			roleResourceType,
			role.ID,
			[]resource.RoleTraitOption{},
		)
		if err != nil {
			return nil, nil, fmt.Errorf("wiz-connector: failed to create role resource: %w", err)
		}

		resources = append(resources, roleResource)
	}

	// Prepare the sync results with next page token if there are more pages
	syncResults := &resource.SyncOpResults{}
	if resp.PageInfo.HasNextPage {
		syncResults.NextPageToken = resp.PageInfo.EndCursor
	}

	return resources, syncResults, nil
}

// StaticEntitlements returns a static "member" entitlement template for all roles.
// The SDK will expand this template for each role resource, constructing the ID, display name, and description.
func (r *roleBuilder) StaticEntitlements(ctx context.Context, _ resource.SyncOpAttrs) ([]*v2.Entitlement, *resource.SyncOpResults, error) {
	var entitlements []*v2.Entitlement
	entitlements = append(
		entitlements,
		ent.NewAssignmentEntitlement(
			nil,
			"member",
			ent.WithDisplayName("Role Member"),
			ent.WithDescription("Member of a Wiz role"),
			ent.WithGrantableTo(userResourceType),
		),
	)

	return entitlements, nil, nil
}

// Entitlements is required by ResourceSyncerV2 but we use StaticEntitlements instead.
// This should not be called due to the SkipEntitlements annotation on the resource type.
func (r *roleBuilder) Entitlements(ctx context.Context, res *v2.Resource, _ resource.SyncOpAttrs) ([]*v2.Entitlement, *resource.SyncOpResults, error) {
	return nil, nil, nil
}

// Grants returns grants for users who have this role.
// Role grants are now emitted from the user resource type to avoid querying all users for each role.
// See users.go Grants() method.
func (r *roleBuilder) Grants(ctx context.Context, res *v2.Resource, attr resource.SyncOpAttrs) ([]*v2.Grant, *resource.SyncOpResults, error) {
	return nil, nil, nil
}

// Grant assigns the role to the user. Wiz users hold exactly one role, so
// this replaces the user's current role; the old role grant drops out on the
// next sync.
func (r *roleBuilder) Grant(ctx context.Context, principal *v2.Resource, entitlement *v2.Entitlement) ([]*v2.Grant, annotations.Annotations, error) {
	if slug := entitlementSlug(entitlement); slug != memberEntitlement {
		return nil, nil, status.Errorf(codes.InvalidArgument, "wiz-connector: unsupported role entitlement %q", slug)
	}
	roleID := entitlement.GetResource().GetId().GetResource()
	if roleID == "" {
		return nil, nil, status.Error(codes.InvalidArgument, "wiz-connector: entitlement has no role ID")
	}

	user, err := resolveUser(ctx, r.client, principal.GetId(), principal)
	if err != nil {
		return nil, nil, err
	}

	unlock := r.client.LockObject("user:" + user.ID)
	defer unlock()

	g := grant.NewGrant(entitlement.GetResource(), memberEntitlement, principal.GetId())
	if user.EffectiveRole.ID == roleID {
		return []*v2.Grant{g}, annotations.New(&v2.GrantAlreadyExists{}), nil
	}

	if err := r.client.UpdateUserRole(ctx, user.ID, roleID); err != nil {
		return nil, nil, fmt.Errorf("wiz-connector: failed to assign role %q to user %q: %w", roleID, user.ID, err)
	}

	return []*v2.Grant{g}, nil, nil
}

// Revoke removes the role from the user. Wiz requires every user to hold a
// role, so a held role can only be replaced by granting a different one;
// revoking a role the user no longer holds succeeds as already revoked.
func (r *roleBuilder) Revoke(ctx context.Context, g *v2.Grant) (annotations.Annotations, error) {
	roleID := g.GetEntitlement().GetResource().GetId().GetResource()
	if roleID == "" {
		return nil, status.Error(codes.InvalidArgument, "wiz-connector: grant has no role ID")
	}

	user, err := resolveUser(ctx, r.client, g.GetPrincipal().GetId(), g.GetPrincipal())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return annotations.New(&v2.GrantAlreadyRevoked{}), nil
		}
		return nil, err
	}

	if user.EffectiveRole.ID != roleID {
		return annotations.New(&v2.GrantAlreadyRevoked{}), nil
	}

	return nil, status.Errorf(codes.FailedPrecondition,
		"wiz-connector: every Wiz user must hold a role, so role %q cannot be removed from user %q; grant the user a different role instead",
		roleID, user.Email)
}

func newRoleBuilder(client wiz.Client) *roleBuilder {
	return &roleBuilder{client: client}
}

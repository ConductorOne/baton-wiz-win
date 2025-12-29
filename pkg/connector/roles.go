package connector

import (
	"context"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/types/entitlement"
	"github.com/conductorone/baton-sdk/pkg/types/grant"
	"github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-wiz-win/pkg/wiz"
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

	for _, edge := range resp.Edges {
		role := edge.Node

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

// Entitlements returns a "member" entitlement for each role.
func (r *roleBuilder) Entitlements(ctx context.Context, res *v2.Resource, _ resource.SyncOpAttrs) ([]*v2.Entitlement, *resource.SyncOpResults, error) {
	var entitlements []*v2.Entitlement

	// Create a "member" entitlement for the role
	memberEntitlement := entitlement.NewAssignmentEntitlement(
		res,
		"member",
		entitlement.WithGrantableTo(userResourceType),
		entitlement.WithDisplayName(fmt.Sprintf("%s Role Member", res.DisplayName)),
		entitlement.WithDescription(fmt.Sprintf("Access to %s role in Wiz", res.DisplayName)),
	)

	entitlements = append(entitlements, memberEntitlement)

	return entitlements, nil, nil
}

// Grants returns grants for users who have this role, one page at a time.
// In Wiz, role assignments are typically associated with users directly,
// so we fetch users and check their roles.
func (r *roleBuilder) Grants(ctx context.Context, res *v2.Resource, attr resource.SyncOpAttrs) ([]*v2.Grant, *resource.SyncOpResults, error) {
	var grants []*v2.Grant

	// Get the page token from the sync attributes
	var cursor *string
	if attr.PageToken.Token != "" {
		cursor = &attr.PageToken.Token
	}

	roleID := res.Id.Resource

	// Fetch one page of users
	resp, err := r.client.ListUsers(ctx, cursor)
	if err != nil {
		return nil, nil, fmt.Errorf("wiz-connector: failed to list users for role grants: %w", err)
	}

	for _, edge := range resp.Edges {
		user := edge.Node

		// Check if user has this role
		if user.Role.ID == roleID || user.Role.Name == res.DisplayName {
			userResource, err := resource.NewResourceID(userResourceType, user.ID)
			if err != nil {
				return nil, nil, fmt.Errorf("wiz-connector: failed to create user resource ID: %w", err)
			}

			g := grant.NewGrant(
				res,
				"member",
				userResource,
			)

			grants = append(grants, g)
		}
	}

	// Prepare the sync results with next page token if there are more pages
	syncResults := &resource.SyncOpResults{}
	if resp.PageInfo.HasNextPage {
		syncResults.NextPageToken = resp.PageInfo.EndCursor
	}

	return grants, syncResults, nil
}

func newRoleBuilder(client wiz.Client) *roleBuilder {
	return &roleBuilder{client: client}
}

package connector

import (
	"context"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/types/entitlement"
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

// Grants returns grants for users who have this role.
// NOTE: This is currently non-functional because:
// - The 'users' endpoint (which has effectiveRole) requires user session auth
// - Service accounts can only access 'userAccounts' which lacks role information
// - There is no API endpoint to query users by role
// This is a fundamental limitation of the Wiz API for service account authentication.
func (r *roleBuilder) Grants(ctx context.Context, res *v2.Resource, attr resource.SyncOpAttrs) ([]*v2.Grant, *resource.SyncOpResults, error) {
	// Cannot determine user-to-role mappings without access to the 'users' endpoint
	// which requires user session authentication (not available to service accounts).
	//
	// Alternative endpoints tested:
	// - userAccounts: no effectiveRole field
	// - directoryUsers: no effectiveRole field
	// - userRolesV2: no assignedUsers field
	//
	// TODO: Once Wiz provides API access to user-role mappings for service accounts,
	// implement the grants logic here.
	return nil, nil, nil
}

func newRoleBuilder(client wiz.Client) *roleBuilder {
	return &roleBuilder{client: client}
}

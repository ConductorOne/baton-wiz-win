package connector

import (
	"context"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/types/grant"
	"github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-wiz-win/pkg/wiz"
)

type userBuilder struct {
	client wiz.Client
}

func (u *userBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return userResourceType
}

// List returns users from Wiz as resource objects, one page at a time.
func (u *userBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, attr resource.SyncOpAttrs) ([]*v2.Resource, *resource.SyncOpResults, error) {
	var users []*v2.Resource

	// Get the page token from the sync attributes
	var cursor *string
	if attr.PageToken.Token != "" {
		cursor = &attr.PageToken.Token
	}

	// Fetch one page of users
	resp, err := u.client.ListUsers(ctx, cursor)
	if err != nil {
		return nil, nil, fmt.Errorf("wiz-connector: failed to list users: %w", err)
	}

	for _, user := range resp.Nodes {
		// Skip users without email addresses (can't use them as resource IDs)
		if user.Email == "" {
			continue
		}

		// Use email as the resource ID instead of the Wiz user ID because:
		// - userAccounts and users endpoints return different IDs for the same person
		// - Email is consistent across all Wiz API endpoints
		// - Project grants reference users by email
		userResource, err := resource.NewUserResource(
			user.Email,
			userResourceType,
			user.Email, // Use email as ID for consistency
			[]resource.UserTraitOption{
				resource.WithEmail(user.Email, true),
				resource.WithStatus(v2.UserTrait_Status_STATUS_ENABLED),
			},
		)
		if err != nil {
			return nil, nil, fmt.Errorf("wiz-connector: failed to create user resource: %w", err)
		}

		users = append(users, userResource)
	}

	// Prepare the sync results with next page token if there are more pages
	syncResults := &resource.SyncOpResults{}
	if resp.PageInfo.HasNextPage {
		syncResults.NextPageToken = resp.PageInfo.EndCursor
	}

	return users, syncResults, nil
}

// Entitlements returns an empty slice for users as they don't have child entitlements.
func (u *userBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ resource.SyncOpAttrs) ([]*v2.Entitlement, *resource.SyncOpResults, error) {
	return nil, nil, nil
}

// Grants returns grants for projects this user is a member of.
// This uses the effectiveAssignedProjects field from the users query.
func (u *userBuilder) Grants(ctx context.Context, res *v2.Resource, attr resource.SyncOpAttrs) ([]*v2.Grant, *resource.SyncOpResults, error) {
	var grants []*v2.Grant

	// Get the page token from the sync attributes
	var cursor *string
	if attr.PageToken.Token != "" {
		cursor = &attr.PageToken.Token
	}

	userEmail := res.Id.Resource

	// Fetch users to find the specific user and their project assignments
	resp, err := u.client.ListUsers(ctx, cursor)
	if err != nil {
		return nil, nil, fmt.Errorf("wiz-connector: failed to list users for project grants: %w", err)
	}

	// Find the specific user we're getting grants for
	for _, user := range resp.Nodes {
		if user.Email != userEmail {
			continue
		}

		// Create grants for each project the user is assigned to
		for _, project := range user.EffectiveAssignedProjects {
			// Create the project resource for the grant
			projectRes, err := resource.NewGroupResource(
				project.Name,
				projectResourceType,
				project.ID,
				[]resource.GroupTraitOption{},
			)
			if err != nil {
				return nil, nil, fmt.Errorf("wiz-connector: failed to create project resource: %w", err)
			}

			// Create a grant for this user to the project's "member" entitlement
			userGrant := grant.NewGrant(projectRes, "member", res.Id)

			grants = append(grants, userGrant)
		}

		// Found the user, no need to continue
		break
	}

	// Prepare the sync results with next page token if there are more pages
	syncResults := &resource.SyncOpResults{}
	if resp.PageInfo.HasNextPage {
		syncResults.NextPageToken = resp.PageInfo.EndCursor
	}

	return grants, syncResults, nil
}

func newUserBuilder(client wiz.Client) *userBuilder {
	return &userBuilder{client: client}
}

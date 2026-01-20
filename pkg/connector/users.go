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

		// Store role ID and project IDs in profile for use in Grants()
		// This avoids having to query all users again when generating grants
		profile := make(map[string]interface{})
		if user.EffectiveRole.ID != "" {
			profile["role_id"] = user.EffectiveRole.ID
		}

		projectIDs := make([]interface{}, 0, len(user.EffectiveAssignedProjects))
		for _, project := range user.EffectiveAssignedProjects {
			projectIDs = append(projectIDs, project.ID)
		}
		if len(projectIDs) > 0 {
			profile["project_ids"] = projectIDs
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
				resource.WithUserProfile(profile),
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

// Grants returns grants for projects this user is a member of and their role assignment.
// Data is read from the user profile that was populated during List().
func (u *userBuilder) Grants(ctx context.Context, res *v2.Resource, attr resource.SyncOpAttrs) ([]*v2.Grant, *resource.SyncOpResults, error) {
	var grants []*v2.Grant

	// Extract the user trait to get the profile data
	userTrait, err := resource.GetUserTrait(res)
	if err != nil {
		return nil, nil, fmt.Errorf("wiz-connector: failed to get user trait: %w", err)
	}

	profile := userTrait.GetProfile()
	if profile == nil {
		// No profile data, return empty grants
		return grants, nil, nil
	}

	// Create role grant if role_id is present
	if roleID, ok := profile.Fields["role_id"]; ok && roleID.GetStringValue() != "" {
		roleResource, err := resource.NewRoleResource(
			"", // Name is not needed for grant creation
			roleResourceType,
			roleID.GetStringValue(),
			[]resource.RoleTraitOption{},
		)
		if err != nil {
			return nil, nil, fmt.Errorf("wiz-connector: failed to create role resource: %w", err)
		}

		roleGrant := grant.NewGrant(roleResource, "member", res.Id)
		grants = append(grants, roleGrant)
	}

	// Create project grants if project_ids is present
	if projectIDsValue, ok := profile.Fields["project_ids"]; ok {
		projectIDsList := projectIDsValue.GetListValue()
		if projectIDsList != nil {
			for _, projectIDValue := range projectIDsList.Values {
				projectID := projectIDValue.GetStringValue()
				if projectID == "" {
					continue
				}

				projectRes, err := resource.NewGroupResource(
					"", // Name is not needed for grant creation
					projectResourceType,
					projectID,
					[]resource.GroupTraitOption{},
				)
				if err != nil {
					return nil, nil, fmt.Errorf("wiz-connector: failed to create project resource: %w", err)
				}

				projectGrant := grant.NewGrant(projectRes, "member", res.Id)
				grants = append(grants, projectGrant)
			}
		}
	}

	return grants, nil, nil
}

func newUserBuilder(client wiz.Client) *userBuilder {
	return &userBuilder{client: client}
}

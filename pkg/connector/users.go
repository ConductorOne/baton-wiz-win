package connector

import (
	"context"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
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

	for _, edge := range resp.Edges {
		user := edge.Node

		userResource, err := resource.NewUserResource(
			user.Email,
			userResourceType,
			user.ID,
			[]resource.UserTraitOption{
				resource.WithEmail(user.Email, true),
				resource.WithStatus(v2.UserTrait_Status_STATUS_ENABLED),
			},
			resource.WithDescription(fmt.Sprintf("Wiz user with role: %s (%s)", user.Role.Name, user.Role.ID)),
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

// Grants returns an empty slice for users. Role and project memberships are handled
// by the role and project builders.
func (u *userBuilder) Grants(ctx context.Context, resource *v2.Resource, attr resource.SyncOpAttrs) ([]*v2.Grant, *resource.SyncOpResults, error) {
	return nil, nil, nil
}

func newUserBuilder(client wiz.Client) *userBuilder {
	return &userBuilder{client: client}
}

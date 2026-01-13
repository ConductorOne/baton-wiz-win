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

type projectBuilder struct {
	client wiz.Client
}

func (p *projectBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return projectResourceType
}

// List returns projects from Wiz as resource objects, one page at a time.
func (p *projectBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, attr resource.SyncOpAttrs) ([]*v2.Resource, *resource.SyncOpResults, error) {
	var projects []*v2.Resource

	// Get the page token from the sync attributes
	var cursor *string
	if attr.PageToken.Token != "" {
		cursor = &attr.PageToken.Token
	}

	// Fetch one page of projects
	resp, err := p.client.ListProjects(ctx, cursor)
	if err != nil {
		return nil, nil, fmt.Errorf("wiz-connector: failed to list projects: %w", err)
	}

	for _, edge := range resp.Edges {
		project := edge.Node

		projectResource, err := resource.NewGroupResource(
			project.Name,
			projectResourceType,
			project.ID,
			[]resource.GroupTraitOption{},
			resource.WithDescription(project.Description),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("wiz-connector: failed to create project resource: %w", err)
		}

		projects = append(projects, projectResource)
	}

	// Prepare the sync results with next page token if there are more pages
	syncResults := &resource.SyncOpResults{}
	if resp.PageInfo.HasNextPage {
		syncResults.NextPageToken = resp.PageInfo.EndCursor
	}

	return projects, syncResults, nil
}

// Entitlements returns a "member" entitlement for each project.
func (p *projectBuilder) Entitlements(ctx context.Context, res *v2.Resource, _ resource.SyncOpAttrs) ([]*v2.Entitlement, *resource.SyncOpResults, error) {
	var entitlements []*v2.Entitlement

	// Create a "member" entitlement for the project
	memberEntitlement := entitlement.NewAssignmentEntitlement(
		res,
		"member",
		entitlement.WithGrantableTo(userResourceType),
		entitlement.WithDisplayName(fmt.Sprintf("%s Project Member", res.DisplayName)),
		entitlement.WithDescription(fmt.Sprintf("Membership in %s project", res.DisplayName)),
	)

	entitlements = append(entitlements, memberEntitlement)

	return entitlements, nil, nil
}

// Grants returns grants for users who are members of this project.
// Wiz projects have projectOwners and securityChampions.
func (p *projectBuilder) Grants(ctx context.Context, res *v2.Resource, attr resource.SyncOpAttrs) ([]*v2.Grant, *resource.SyncOpResults, error) {
	var grants []*v2.Grant

	// Get the page token from the sync attributes
	var cursor *string
	if attr.PageToken.Token != "" {
		cursor = &attr.PageToken.Token
	}

	projectID := res.Id.Resource

	// Fetch one page of projects
	resp, err := p.client.ListProjects(ctx, cursor)
	if err != nil {
		return nil, nil, fmt.Errorf("wiz-connector: failed to list projects for grants: %w", err)
	}

	// Find the specific project we're getting grants for
	for _, edge := range resp.Edges {
		project := edge.Node

		if project.ID != projectID {
			continue
		}

		// Create grants for project owners
		// Use email as the user ID to match how we sync users (email is consistent across endpoints)
		for _, owner := range project.ProjectOwners {
			if owner.Email == "" {
				continue // Skip if no email
			}
			userResource, err := resource.NewResourceID(userResourceType, owner.Email)
			if err != nil {
				return nil, nil, fmt.Errorf("wiz-connector: failed to create user resource ID for owner: %w", err)
			}

			g := grant.NewGrant(
				res,
				"member",
				userResource,
			)
			grants = append(grants, g)
		}

		// Create grants for security champions
		for _, champion := range project.SecurityChampions {
			if champion.Email == "" {
				continue // Skip if no email
			}
			userResource, err := resource.NewResourceID(userResourceType, champion.Email)
			if err != nil {
				return nil, nil, fmt.Errorf("wiz-connector: failed to create user resource ID for champion: %w", err)
			}

			g := grant.NewGrant(
				res,
				"member",
				userResource,
			)
			grants = append(grants, g)
		}

		// Found the project, no need to continue
		break
	}

	// Prepare the sync results with next page token if there are more pages
	// Note: This will continue paginating through all projects until we find the one we need
	syncResults := &resource.SyncOpResults{}
	if resp.PageInfo.HasNextPage {
		syncResults.NextPageToken = resp.PageInfo.EndCursor
	}

	return grants, syncResults, nil
}

func newProjectBuilder(client wiz.Client) *projectBuilder {
	return &projectBuilder{client: client}
}

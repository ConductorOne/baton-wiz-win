package connector

import (
	"context"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
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

	for _, project := range resp.Nodes {
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

// StaticEntitlements returns static "owner", "champion", and "member" entitlements for all projects.
// This is called once per resource type, not per resource.
func (p *projectBuilder) StaticEntitlements(ctx context.Context, _ resource.SyncOpAttrs) ([]*v2.Entitlement, *resource.SyncOpResults, error) {
	var entitlements []*v2.Entitlement

	entitlements = append(
		entitlements,
		ent.NewAssignmentEntitlement(
			nil,
			"owner",
			ent.WithDisplayName("Project Owner"),
			ent.WithDescription("Owner of a Wiz project with full administrative access"),
			ent.WithGrantableTo(userResourceType),
		),
		ent.NewAssignmentEntitlement(
			nil,
			"champion",
			ent.WithDisplayName("Security Champion"),
			ent.WithDescription("Security champion for a Wiz project"),
			ent.WithGrantableTo(userResourceType),
		),
		ent.NewAssignmentEntitlement(
			nil,
			"member",
			ent.WithDisplayName("Project Member"),
			ent.WithDescription("General member of a Wiz project"),
			ent.WithGrantableTo(userResourceType),
		),
	)

	return entitlements, nil, nil
}

// Entitlements is required by ResourceSyncerV2 but we use StaticEntitlements instead.
// This should not be called due to the SkipEntitlements annotation on the resource type.
func (p *projectBuilder) Entitlements(ctx context.Context, res *v2.Resource, _ resource.SyncOpAttrs) ([]*v2.Entitlement, *resource.SyncOpResults, error) {
	return nil, nil, nil
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
	for _, project := range resp.Nodes {
		if project.ID != projectID {
			continue
		}

		// Create grants for project owners with "owner" entitlement
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
				"owner",
				userResource,
			)
			grants = append(grants, g)
		}

		// Create grants for security champions with "champion" entitlement
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
				"champion",
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

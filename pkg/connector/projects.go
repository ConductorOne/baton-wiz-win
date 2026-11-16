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

// Grant adds the user to the project as a member, owner, or security champion.
func (p *projectBuilder) Grant(ctx context.Context, principal *v2.Resource, entitlement *v2.Entitlement) ([]*v2.Grant, annotations.Annotations, error) {
	slug := entitlementSlug(entitlement)
	projectID := entitlement.GetResource().GetId().GetResource()
	if projectID == "" {
		return nil, nil, status.Error(codes.InvalidArgument, "wiz-connector: entitlement has no project ID")
	}

	user, err := resolveUser(ctx, p.client, principal.GetId(), principal)
	if err != nil {
		return nil, nil, err
	}

	var exists bool
	switch slug {
	case memberEntitlement:
		exists, err = p.addProjectAssignment(ctx, user, projectID)
	case ownerEntitlement:
		exists, err = p.addProjectMember(ctx, user, projectID, wiz.ProjectOwnersField)
	case championEntitlement:
		exists, err = p.addProjectMember(ctx, user, projectID, wiz.SecurityChampionsField)
	default:
		return nil, nil, status.Errorf(codes.InvalidArgument, "wiz-connector: unsupported project entitlement %q", slug)
	}
	if err != nil {
		return nil, nil, err
	}

	g := grant.NewGrant(entitlement.GetResource(), slug, principal.GetId())
	if exists {
		return []*v2.Grant{g}, annotations.New(&v2.GrantAlreadyExists{}), nil
	}
	return []*v2.Grant{g}, nil, nil
}

// Revoke removes the user from the project's members, owners, or security champions.
func (p *projectBuilder) Revoke(ctx context.Context, g *v2.Grant) (annotations.Annotations, error) {
	slug := entitlementSlug(g.GetEntitlement())
	projectID := g.GetEntitlement().GetResource().GetId().GetResource()
	if projectID == "" {
		return nil, status.Error(codes.InvalidArgument, "wiz-connector: grant has no project ID")
	}

	user, err := resolveUser(ctx, p.client, g.GetPrincipal().GetId(), g.GetPrincipal())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return annotations.New(&v2.GrantAlreadyRevoked{}), nil
		}
		return nil, err
	}

	var revoked bool
	switch slug {
	case memberEntitlement:
		revoked, err = p.removeProjectAssignment(ctx, user, projectID)
	case ownerEntitlement:
		revoked, err = p.removeProjectMember(ctx, user, projectID, wiz.ProjectOwnersField)
	case championEntitlement:
		revoked, err = p.removeProjectMember(ctx, user, projectID, wiz.SecurityChampionsField)
	default:
		return nil, status.Errorf(codes.InvalidArgument, "wiz-connector: unsupported project entitlement %q", slug)
	}
	if err != nil {
		return nil, err
	}
	if revoked {
		return annotations.New(&v2.GrantAlreadyRevoked{}), nil
	}
	return nil, nil
}

// addProjectAssignment adds the project to the user's assigned projects and
// reports whether it was already assigned. Assigned projects only scope users
// with a project-scoped role; a global role already covers every project.
func (p *projectBuilder) addProjectAssignment(ctx context.Context, user *wiz.UserDetail, projectID string) (bool, error) {
	unlock := p.client.LockObject("user:" + user.ID)
	defer unlock()

	// Re-read under the lock so a concurrent change to the same user is not overwritten.
	current, err := p.client.GetUser(ctx, user.ID)
	if err != nil {
		return false, fmt.Errorf("wiz-connector: failed to get user %q: %w", user.ID, err)
	}
	if current == nil {
		return false, status.Errorf(codes.NotFound, "wiz-connector: user %q no longer exists", user.ID)
	}

	assigned := projectIDs(current.EffectiveAssignedProjects)
	if containsString(assigned, projectID) {
		return true, nil
	}
	if !current.EffectiveRole.IsProjectScoped {
		return false, status.Errorf(codes.FailedPrecondition,
			"wiz-connector: user %q has the global role %q, which already covers every project; grant a project-scoped role before assigning projects",
			current.Email, current.EffectiveRole.Name)
	}

	if err := p.client.UpdateUserProjects(ctx, current.ID, append(assigned, projectID)); err != nil {
		return false, fmt.Errorf("wiz-connector: failed to assign project %q to user %q: %w", projectID, current.ID, err)
	}
	return false, nil
}

// removeProjectAssignment removes the project from the user's assigned
// projects and reports whether it was already absent.
func (p *projectBuilder) removeProjectAssignment(ctx context.Context, user *wiz.UserDetail, projectID string) (bool, error) {
	unlock := p.client.LockObject("user:" + user.ID)
	defer unlock()

	current, err := p.client.GetUser(ctx, user.ID)
	if err != nil {
		return false, fmt.Errorf("wiz-connector: failed to get user %q: %w", user.ID, err)
	}
	if current == nil {
		return true, nil
	}

	assigned := projectIDs(current.EffectiveAssignedProjects)
	if !containsString(assigned, projectID) {
		return true, nil
	}

	if err := p.client.UpdateUserProjects(ctx, current.ID, removeString(assigned, projectID)); err != nil {
		return false, fmt.Errorf("wiz-connector: failed to remove project %q from user %q: %w", projectID, current.ID, err)
	}
	return false, nil
}

// addProjectMember adds the user to the project's owners or security
// champions and reports whether the user was already listed.
func (p *projectBuilder) addProjectMember(ctx context.Context, user *wiz.UserDetail, projectID string, field wiz.ProjectMemberField) (bool, error) {
	unlock := p.client.LockObject("project:" + projectID)
	defer unlock()

	project, err := p.client.GetProject(ctx, projectID)
	if err != nil {
		return false, fmt.Errorf("wiz-connector: failed to get project %q: %w", projectID, err)
	}
	if project == nil {
		return false, status.Errorf(codes.NotFound, "wiz-connector: project %q not found", projectID)
	}

	ids, found := memberIDs(projectMembers(project, field), user)
	if found {
		return true, nil
	}

	if err := p.client.UpdateProjectMembers(ctx, projectID, field, append(ids, user.ID)); err != nil {
		return false, fmt.Errorf("wiz-connector: failed to add user %q to project %q: %w", user.ID, projectID, err)
	}
	return false, nil
}

// removeProjectMember removes the user from the project's owners or security
// champions and reports whether the user was already absent.
func (p *projectBuilder) removeProjectMember(ctx context.Context, user *wiz.UserDetail, projectID string, field wiz.ProjectMemberField) (bool, error) {
	unlock := p.client.LockObject("project:" + projectID)
	defer unlock()

	project, err := p.client.GetProject(ctx, projectID)
	if err != nil {
		return false, fmt.Errorf("wiz-connector: failed to get project %q: %w", projectID, err)
	}
	if project == nil {
		return true, nil
	}

	members := projectMembers(project, field)
	if _, found := memberIDs(members, user); !found {
		return true, nil
	}

	if err := p.client.UpdateProjectMembers(ctx, projectID, field, removeMember(members, user)); err != nil {
		return false, fmt.Errorf("wiz-connector: failed to remove user %q from project %q: %w", user.ID, projectID, err)
	}
	return false, nil
}

func projectMembers(project *wiz.ProjectDetail, field wiz.ProjectMemberField) []wiz.ProjectMember {
	if field == wiz.ProjectOwnersField {
		return project.ProjectOwners
	}
	return project.SecurityChampions
}

func newProjectBuilder(client wiz.Client) *projectBuilder {
	return &projectBuilder{client: client}
}

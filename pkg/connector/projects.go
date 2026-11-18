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

// projectBuilder syncs Wiz projects. Owner and champion grants come from the
// project; member grants are emitted by userBuilder from each user's assigned
// projects.
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
		// Owners and champions are kept on the profile so Grants needs no
		// per-project lookup. They are stored by email because the user
		// resource ID is the email.
		owners := make([]interface{}, 0, len(project.ProjectOwners))
		for _, o := range project.ProjectOwners {
			if o.Email != "" {
				owners = append(owners, o.Email)
			}
		}
		champions := make([]interface{}, 0, len(project.SecurityChampions))
		for _, c := range project.SecurityChampions {
			if c.Email != "" {
				champions = append(champions, c.Email)
			}
		}
		profile := map[string]interface{}{
			profileOwnerEmailsKey:    owners,
			profileChampionEmailsKey: champions,
		}

		projectResource, err := resource.NewGroupResource(
			project.Name,
			projectResourceType,
			project.ID,
			[]resource.GroupTraitOption{},
			resource.WithDescription(project.Description),
			resource.WithResourceProfile(profile),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("wiz-connector: failed to create project resource: %w", err)
		}

		projects = append(projects, projectResource)
	}

	next, err := wiz.NextPageToken(resp.PageInfo)
	if err != nil {
		return nil, nil, fmt.Errorf("wiz-connector: failed to list projects: %w", err)
	}

	return projects, &resource.SyncOpResults{NextPageToken: next}, nil
}

// StaticEntitlements returns static "owner", "champion", and "member" entitlements for all projects.
// This is called once per resource type, not per resource.
func (p *projectBuilder) StaticEntitlements(ctx context.Context, _ resource.SyncOpAttrs) ([]*v2.Entitlement, *resource.SyncOpResults, error) {
	var entitlements []*v2.Entitlement

	entitlements = append(
		entitlements,
		ent.NewAssignmentEntitlement(
			nil,
			ownerEntitlement,
			ent.WithDisplayName("Project Owner"),
			ent.WithDescription("Owner of a Wiz project with full administrative access"),
			ent.WithGrantableTo(userResourceType),
		),
		ent.NewAssignmentEntitlement(
			nil,
			championEntitlement,
			ent.WithDisplayName("Security Champion"),
			ent.WithDescription("Security champion for a Wiz project"),
			ent.WithGrantableTo(userResourceType),
		),
		ent.NewAssignmentEntitlement(
			nil,
			memberEntitlement,
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

// Grants returns owner and security champion grants for the project, read
// from the profile written during List.
func (p *projectBuilder) Grants(ctx context.Context, res *v2.Resource, attr resource.SyncOpAttrs) ([]*v2.Grant, *resource.SyncOpResults, error) {
	var grants []*v2.Grant
	for _, entry := range []struct {
		key  string
		slug string
	}{
		{profileOwnerEmailsKey, ownerEntitlement},
		{profileChampionEmailsKey, championEntitlement},
	} {
		for _, email := range profileStrings(res, entry.key) {
			userResource, err := resource.NewResourceID(userResourceType, email)
			if err != nil {
				return nil, nil, fmt.Errorf("wiz-connector: failed to create user resource ID for %s: %w", entry.slug, err)
			}
			grants = append(grants, grant.NewGrant(res, entry.slug, userResource))
		}
	}
	return grants, nil, nil
}

// Grant adds the user to the project as a member, owner, or security champion.
func (p *projectBuilder) Grant(ctx context.Context, principal *v2.Resource, entitlement *v2.Entitlement) ([]*v2.Grant, annotations.Annotations, error) {
	slug := entitlementSlug(entitlement)
	projectID := entitlement.GetResource().GetId().GetResource()
	if projectID == "" {
		return nil, nil, status.Error(codes.InvalidArgument, "wiz-connector: entitlement has no project ID")
	}

	user, err := resolveWizUser(ctx, p.client, principal)
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

	user, err := resolveWizUser(ctx, p.client, g.GetPrincipal())
	if err != nil {
		if isUserNotFound(err) {
			return annotations.New(&v2.GrantAlreadyRevoked{}), nil
		}
		return nil, err
	}

	var alreadyAbsent bool
	switch slug {
	case memberEntitlement:
		alreadyAbsent, err = p.removeProjectAssignment(ctx, user, projectID)
	case ownerEntitlement:
		alreadyAbsent, err = p.removeProjectMember(ctx, user, projectID, wiz.ProjectOwnersField)
	case championEntitlement:
		alreadyAbsent, err = p.removeProjectMember(ctx, user, projectID, wiz.SecurityChampionsField)
	default:
		return nil, status.Errorf(codes.InvalidArgument, "wiz-connector: unsupported project entitlement %q", slug)
	}
	if err != nil {
		return nil, err
	}
	if alreadyAbsent {
		return annotations.New(&v2.GrantAlreadyRevoked{}), nil
	}
	return nil, nil
}

// addProjectAssignment adds the project to the user's assigned projects and
// reports whether it was already assigned. Assigned projects only scope users
// with a project-scoped role; a global role already covers every project.
func (p *projectBuilder) addProjectAssignment(ctx context.Context, user *wiz.User, projectID string) (bool, error) {
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
func (p *projectBuilder) removeProjectAssignment(ctx context.Context, user *wiz.User, projectID string) (bool, error) {
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
func (p *projectBuilder) addProjectMember(ctx context.Context, user *wiz.User, projectID string, field wiz.ProjectMemberField) (bool, error) {
	unlock := p.client.LockObject("project:" + projectID)
	defer unlock()

	project, err := p.client.GetProject(ctx, projectID)
	if err != nil {
		return false, fmt.Errorf("wiz-connector: failed to get project %q: %w", projectID, err)
	}
	if project == nil {
		return false, status.Errorf(codes.NotFound, "wiz-connector: project %q not found", projectID)
	}

	others, found, err := otherMemberIDs(projectMembers(project, field), user)
	if err != nil {
		return false, err
	}
	if found {
		return true, nil
	}

	if err := p.client.UpdateProjectMembers(ctx, projectID, field, append(others, user.ID)); err != nil {
		return false, fmt.Errorf("wiz-connector: failed to add user %q to project %q: %w", user.ID, projectID, err)
	}
	return false, nil
}

// removeProjectMember removes the user from the project's owners or security
// champions and reports whether the user was already absent.
func (p *projectBuilder) removeProjectMember(ctx context.Context, user *wiz.User, projectID string, field wiz.ProjectMemberField) (bool, error) {
	unlock := p.client.LockObject("project:" + projectID)
	defer unlock()

	project, err := p.client.GetProject(ctx, projectID)
	if err != nil {
		return false, fmt.Errorf("wiz-connector: failed to get project %q: %w", projectID, err)
	}
	if project == nil {
		return true, nil
	}

	others, found, err := otherMemberIDs(projectMembers(project, field), user)
	if err != nil {
		return false, err
	}
	if !found {
		return true, nil
	}

	if err := p.client.UpdateProjectMembers(ctx, projectID, field, others); err != nil {
		return false, fmt.Errorf("wiz-connector: failed to remove user %q from project %q: %w", user.ID, projectID, err)
	}
	return false, nil
}

func newProjectBuilder(client wiz.Client) *projectBuilder {
	return &projectBuilder{client: client}
}

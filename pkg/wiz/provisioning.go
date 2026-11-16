package wiz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
)

// UserRoleDetail is a role reference that also carries its scoping, which
// decides whether project assignments apply to the user.
type UserRoleDetail struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	IsProjectScoped bool   `json:"isProjectScoped"`
}

// UserDetail is the subset of a Wiz user needed to provision role and
// project assignments.
type UserDetail struct {
	ID                        string         `json:"id"`
	Email                     string         `json:"email"`
	Name                      string         `json:"name"`
	EffectiveRole             UserRoleDetail `json:"effectiveRole"`
	EffectiveAssignedProjects []ProjectRef   `json:"effectiveAssignedProjects"`
}

const userDetailFields = `
	id
	email
	name
	effectiveRole {
		id
		name
		isProjectScoped
	}
	effectiveAssignedProjects {
		id
		name
	}
`

// GetUser returns the user with the given Wiz user ID, or nil when it does not exist.
func (c *client) GetUser(ctx context.Context, userID string) (*UserDetail, error) {
	query := `query GetUser($id: ID!) { user(id: $id) {` + userDetailFields + `} }`

	var result struct {
		User *UserDetail `json:"user"`
	}
	if err := c.graphQLRequest(ctx, query, map[string]interface{}{"id": userID}, &result); err != nil {
		if isGraphQLNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return result.User, nil
}

// FindUserByEmail returns the user whose email matches exactly (case-insensitive),
// or nil when no such user exists. The search filter is a substring match, so
// results are paged through and compared exactly.
func (c *client) FindUserByEmail(ctx context.Context, email string) (*UserDetail, error) {
	query := `query FindUser($first: Int, $after: String, $filterBy: UserFilters) {
		users(first: $first, after: $after, filterBy: $filterBy) {
			nodes {` + userDetailFields + `}
			pageInfo {
				endCursor
				hasNextPage
			}
		}
	}`

	cursor := ""
	for {
		variables := map[string]interface{}{
			"first":    100,
			"filterBy": map[string]interface{}{"search": email},
		}
		if cursor != "" {
			variables["after"] = cursor
		}

		var result struct {
			Users struct {
				Nodes    []UserDetail `json:"nodes"`
				PageInfo PageInfo     `json:"pageInfo"`
			} `json:"users"`
		}
		if err := c.graphQLRequest(ctx, query, variables, &result); err != nil {
			return nil, fmt.Errorf("failed to search users: %w", err)
		}

		for i := range result.Users.Nodes {
			if strings.EqualFold(result.Users.Nodes[i].Email, email) {
				return &result.Users.Nodes[i], nil
			}
		}

		if !result.Users.PageInfo.HasNextPage || result.Users.PageInfo.EndCursor == "" || result.Users.PageInfo.EndCursor == cursor {
			return nil, nil
		}
		cursor = result.Users.PageInfo.EndCursor
	}
}

const updateUserMutation = `mutation UpdateUser($input: UpdateUserInput!) {
	updateUser(input: $input) {
		user {
			id
		}
	}
}`

// UpdateUserRole sets the user's role. Wiz users hold exactly one role, so
// this replaces the current one.
func (c *client) UpdateUserRole(ctx context.Context, userID, roleID string) error {
	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"id":    userID,
			"patch": map[string]interface{}{"role": roleID},
		},
	}
	var result struct{}
	if err := c.graphQLRequest(ctx, updateUserMutation, variables, &result); err != nil {
		return fmt.Errorf("failed to update user role: %w", err)
	}
	return nil
}

// UpdateUserProjects replaces the user's assigned projects. An empty slice
// is sent as an explicit empty list so the last project can be removed.
func (c *client) UpdateUserProjects(ctx context.Context, userID string, projectIDs []string) error {
	if projectIDs == nil {
		projectIDs = []string{}
	}
	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"id":    userID,
			"patch": map[string]interface{}{"assignedProjectIds": projectIDs},
		},
	}
	var result struct{}
	if err := c.graphQLRequest(ctx, updateUserMutation, variables, &result); err != nil {
		return fmt.Errorf("failed to update user projects: %w", err)
	}
	return nil
}

// ProjectMember is a user listed as a project owner or security champion.
type ProjectMember struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// ProjectDetail is the subset of a Wiz project needed to provision owners and champions.
type ProjectDetail struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	ProjectOwners     []ProjectMember `json:"projectOwners"`
	SecurityChampions []ProjectMember `json:"securityChampions"`
}

// GetProject returns the project with the given ID, or nil when it does not exist.
func (c *client) GetProject(ctx context.Context, projectID string) (*ProjectDetail, error) {
	query := `query GetProject($id: ID!) {
		project(id: $id) {
			id
			name
			projectOwners {
				id
				email
			}
			securityChampions {
				id
				email
			}
		}
	}`

	var result struct {
		Project *ProjectDetail `json:"project"`
	}
	if err := c.graphQLRequest(ctx, query, map[string]interface{}{"id": projectID}, &result); err != nil {
		if isGraphQLNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	return result.Project, nil
}

// ProjectMemberField names the project list a user can be added to.
type ProjectMemberField string

const (
	ProjectOwnersField     ProjectMemberField = "projectOwners"
	SecurityChampionsField ProjectMemberField = "securityChampions"
)

// UpdateProjectMembers replaces one member list (owners or security champions)
// on a project. Only that field is patched; the rest of the project is untouched.
func (c *client) UpdateProjectMembers(ctx context.Context, projectID string, field ProjectMemberField, userIDs []string) error {
	if field != ProjectOwnersField && field != SecurityChampionsField {
		return fmt.Errorf("unsupported project member field %q", field)
	}
	if userIDs == nil {
		userIDs = []string{}
	}
	query := `mutation UpdateProject($input: UpdateProjectInput!) {
		updateProject(input: $input) {
			project {
				id
			}
		}
	}`
	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"id":    projectID,
			"patch": map[string]interface{}{string(field): userIDs},
		},
	}
	var result struct{}
	if err := c.graphQLRequest(ctx, query, variables, &result); err != nil {
		return fmt.Errorf("failed to update project %s: %w", field, err)
	}
	return nil
}

// isGraphQLNotFound reports whether Wiz answered with a GraphQL NOT_FOUND
// error. Transport errors such as an HTTP 404 from a wrong API URL also map to
// codes.NotFound, so the check is on the error type, not the status code.
func isGraphQLNotFound(err error) bool {
	var gqlErr *GraphQLError
	return errors.As(err, &gqlErr) && gqlErr.Code == codes.NotFound
}

// keyedLocker serializes read-modify-write updates per Wiz object. The
// mutations replace whole lists, so two concurrent grants on the same user
// or project would otherwise drop one another's change.
type keyedLocker struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func (k *keyedLocker) Lock(key string) func() {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = make(map[string]*sync.Mutex)
	}
	l, ok := k.locks[key]
	if !ok {
		l = &sync.Mutex{}
		k.locks[key] = l
	}
	k.mu.Unlock()

	l.Lock()
	return l.Unlock
}

// LockObject serializes provisioning changes to one Wiz user or project in this process.
func (c *client) LockObject(key string) func() {
	return c.locker.Lock(key)
}

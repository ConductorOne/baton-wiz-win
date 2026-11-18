package wiz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GraphQLError is an error reported in a GraphQL response body. It carries a
// gRPC status classified from extensions.code, since the HTTP status is 200.
// Only the provisioning methods return it; sync methods keep codes.Unknown.
type GraphQLError struct {
	Code   codes.Code
	Errors []graphQLError
}

func (e *GraphQLError) Error() string {
	return fmt.Sprintf("graphql errors: %+v", e.Errors)
}

func (e *GraphQLError) GRPCStatus() *status.Status {
	return status.New(e.Code, e.Error())
}

func graphQLErrorCode(errs []graphQLError) codes.Code {
	switch strings.ToUpper(errs[0].Extensions.Code) {
	case "NOT_FOUND":
		return codes.NotFound
	case "FORBIDDEN", "UNAUTHORIZED", "PERMISSION_DENIED":
		return codes.PermissionDenied
	case "UNAUTHENTICATED":
		return codes.Unauthenticated
	case "BAD_USER_INPUT", "INVALID_INPUT", "GRAPHQL_VALIDATION_FAILED":
		return codes.InvalidArgument
	case "RATE_LIMIT_EXCEEDED", "RATE_LIMITED", "TOO_MANY_REQUESTS", "SERVICE_UNAVAILABLE", "INTERNAL_SERVER_ERROR", "TIMEOUT":
		// Retryable: every mutation here sets a full value, so a retry cannot double-apply.
		return codes.Unavailable
	default:
		return codes.Unknown
	}
}

// isGraphQLNotFound reports whether Wiz answered with a GraphQL NOT_FOUND
// error. A transport 404 (for example a wrong API URL) also maps to
// codes.NotFound, so the check is on the error type, not the status code.
func isGraphQLNotFound(err error) bool {
	var gqlErr *GraphQLError
	return errors.As(err, &gqlErr) && gqlErr.Code == codes.NotFound
}

// graphQLRequestClassified is graphQLRequest with GraphQL errors returned as
// a *GraphQLError.
func (c *client) graphQLRequestClassified(ctx context.Context, query string, variables map[string]interface{}, result interface{}) error {
	gqlErrs, err := c.doGraphQL(ctx, query, variables, result)
	if err != nil {
		return err
	}
	if len(gqlErrs) > 0 {
		return &GraphQLError{Code: graphQLErrorCode(gqlErrs), Errors: gqlErrs}
	}
	return nil
}

// NextPageToken returns the cursor for the next page. A page that claims more
// results without a cursor is an error: ending the sync there would read the
// unlisted objects as deleted.
func NextPageToken(pi PageInfo) (string, error) {
	if !pi.HasNextPage {
		return "", nil
	}
	if pi.EndCursor == "" {
		return "", status.Error(codes.Internal, "wiz reported more results but returned no cursor")
	}
	return pi.EndCursor, nil
}

const provisioningUserFields = `
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

const provisioningProjectFields = `
	id
	name
	description
	projectOwners {
		id
		email
	}
	securityChampions {
		id
		email
	}
`

// GetUser returns the user with the given Wiz user ID, or nil when it does not exist.
func (c *client) GetUser(ctx context.Context, userID string) (*User, error) {
	query := `query GetUser($id: ID!) { user(id: $id) {` + provisioningUserFields + `} }`

	var result struct {
		User *User `json:"user"`
	}
	if err := c.graphQLRequestClassified(ctx, query, map[string]interface{}{"id": userID}, &result); err != nil {
		if isGraphQLNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get wiz user: %w", err)
	}
	return result.User, nil
}

// FindUserByEmail returns the user whose email matches exactly
// (case-insensitive), or nil when no such user exists. The search filter is a
// substring match, so results are paged through and compared exactly. It is
// only a fallback for principals synced without a Wiz user ID.
func (c *client) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `query FindUser($first: Int, $after: String, $filterBy: UserFilters) {
		users(first: $first, after: $after, filterBy: $filterBy) {
			nodes {` + provisioningUserFields + `}
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
			Users UserConnection `json:"users"`
		}
		if err := c.graphQLRequestClassified(ctx, query, variables, &result); err != nil {
			return nil, fmt.Errorf("failed to search wiz users: %w", err)
		}

		for i := range result.Users.Nodes {
			if strings.EqualFold(result.Users.Nodes[i].Email, email) {
				return &result.Users.Nodes[i], nil
			}
		}

		// A broken page must not read as "no such user": revoke would then
		// report the grant as already revoked.
		next, err := NextPageToken(result.Users.PageInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to search wiz users: %w", err)
		}
		if next == "" {
			return nil, nil
		}
		if next == cursor {
			return nil, status.Error(codes.Internal, "failed to search wiz users: wiz returned the same cursor twice")
		}
		cursor = next
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
	if err := c.graphQLRequestClassified(ctx, updateUserMutation, variables, &result); err != nil {
		return fmt.Errorf("failed to update wiz user role: %w", err)
	}
	return nil
}

// UpdateUserProjects replaces the user's assigned projects. An empty slice is
// sent as an explicit empty list so the last project can be removed.
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
	if err := c.graphQLRequestClassified(ctx, updateUserMutation, variables, &result); err != nil {
		return fmt.Errorf("failed to update wiz user projects: %w", err)
	}
	return nil
}

// GetProject returns the project with the given ID, or nil when it does not exist.
func (c *client) GetProject(ctx context.Context, projectID string) (*Project, error) {
	query := `query GetProject($id: ID!) { project(id: $id) {` + provisioningProjectFields + `} }`

	var result struct {
		Project *Project `json:"project"`
	}
	if err := c.graphQLRequestClassified(ctx, query, map[string]interface{}{"id": projectID}, &result); err != nil {
		if isGraphQLNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get wiz project: %w", err)
	}
	return result.Project, nil
}

// ProjectMemberField names a project list a user can be added to.
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
	if err := c.graphQLRequestClassified(ctx, query, variables, &result); err != nil {
		return fmt.Errorf("failed to update wiz project %s: %w", field, err)
	}
	return nil
}

// keyedLocker serializes read-modify-write updates per Wiz object. The
// mutations replace whole lists, so two concurrent grants on the same user or
// project would otherwise drop one another's change.
type keyedLocker struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func (k *keyedLocker) lock(key string) func() {
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
	return c.locker.lock(key)
}

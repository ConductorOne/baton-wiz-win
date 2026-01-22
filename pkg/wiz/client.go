package wiz

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/conductorone/baton-sdk/pkg/uhttp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Client defines the interface for interacting with the Wiz API.
type Client interface {
	ListUsers(ctx context.Context, cursor *string) (*UserConnection, error)
	ListProjects(ctx context.Context, cursor *string) (*ProjectConnection, error)
	ListUserRoles(ctx context.Context, cursor *string) (*UserRoleConnection, error)
	ListIssues(ctx context.Context, cursor *string) (*IssueConnection, error)
}

// client implements the Client interface.
type client struct {
	wrapper *uhttp.BaseHttpClient
	apiURL  string
}

// NewClient creates a new Wiz API client with OAuth2 authentication.
func NewClient(ctx context.Context, apiURL, clientID, clientSecret, authEndpoint string) (Client, error) {

	// Configure OAuth2 client credentials flow
	// Wiz requires the "audience=wiz-api" parameter for token requests
	config := clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     authEndpoint,
		AuthStyle:    oauth2.AuthStyleInParams,
		EndpointParams: map[string][]string{
			"audience": {"wiz-api"},
		},
	}

	// Create an HTTP client that automatically handles token management
	httpClient := config.Client(ctx)

	// Wrap with baton-sdk's HTTP client wrapper for proper error handling and retries
	wrapper, err := uhttp.NewBaseHttpClientWithContext(ctx, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create http client wrapper: %w", err)
	}

	return &client{
		wrapper: wrapper,
		apiURL:  apiURL,
	}, nil
}

// graphQLRequest makes a GraphQL request to the Wiz API using baton-sdk's HTTP wrapper.
// The wrapper handles retries, rate limiting, and error wrapping automatically.
func (c *client) graphQLRequest(ctx context.Context, query string, variables map[string]interface{}, result interface{}) error {
	requestBody := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	// Parse the API URL
	parsedURL, err := url.Parse(c.apiURL)
	if err != nil {
		return fmt.Errorf("failed to parse API URL: %w", err)
	}

	req, err := c.wrapper.NewRequest(ctx, http.MethodPost, parsedURL, uhttp.WithJSONBody(requestBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Use a temporary struct to capture the GraphQL response envelope
	var gqlResp graphQLResponse
	gqlResp.Data = result

	// Execute the request with JSON response handling
	resp, err := c.wrapper.Do(req, uhttp.WithJSONResponse(&gqlResp))
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	// Check for GraphQL-specific errors in the response
	if len(gqlResp.Errors) > 0 {
		return status.Errorf(codes.Unknown, "graphql errors: %+v", gqlResp.Errors)
	}

	return nil
}

// ListUsers retrieves a paginated list of users from Wiz.
// Note: Uses users endpoint which requires read:users permission and includes role and project information.
func (c *client) ListUsers(ctx context.Context, cursor *string) (*UserConnection, error) {
	query := `
		query ListUsers($first: Int, $after: String) {
			users(first: $first, after: $after) {
				nodes {
					id
					name
					email
					effectiveRole {
						id
						name
					}
					effectiveAssignedProjects {
						id
						name
					}
				}
				pageInfo {
					endCursor
					hasNextPage
				}
			}
		}
	`

	variables := map[string]interface{}{
		"first": 100,
	}
	if cursor != nil && *cursor != "" {
		variables["after"] = *cursor
	}

	var result struct {
		Users UserConnection `json:"users"`
	}
	if err := c.graphQLRequest(ctx, query, variables, &result); err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	return &result.Users, nil
}

// ListProjects retrieves a paginated list of projects from Wiz.
func (c *client) ListProjects(ctx context.Context, cursor *string) (*ProjectConnection, error) {
	query := `
		query ListProjects($cursor: String) {
			projects(first: 100, after: $cursor) {
				nodes {
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
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}
	`

	variables := map[string]interface{}{}
	if cursor != nil && *cursor != "" {
		variables["cursor"] = *cursor
	}

	var result projectsQueryResponse
	if err := c.graphQLRequest(ctx, query, variables, &result); err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	return &result.Projects, nil
}

// ListUserRoles retrieves all user roles from Wiz using userRolesV2.
// Note: userRolesV2 doesn't require specific permissions - any valid service account can query it.
func (c *client) ListUserRoles(ctx context.Context, cursor *string) (*UserRoleConnection, error) {
	query := `
		query ListUserRoles($filterBy: UserRoleFilters) {
			userRolesV2(filterBy: $filterBy) {
				id
				name
				description
				scopes
				builtin
				isProjectScoped
			}
		}
	`

	variables := map[string]interface{}{
		"filterBy": map[string]interface{}{},
	}

	var result struct {
		UserRolesV2 []UserRole `json:"userRolesV2"`
	}
	if err := c.graphQLRequest(ctx, query, variables, &result); err != nil {
		return nil, fmt.Errorf("failed to list user roles: %w", err)
	}

	// Convert the array response to UserRoleConnection format
	// Note: userRolesV2 returns a plain array, not a Relay connection
	connection := &UserRoleConnection{
		Nodes: result.UserRolesV2,
	}

	return connection, nil
}

// ListIssues retrieves a paginated list of security issues from Wiz.
// Only returns issues affecting USER_ACCOUNT or SERVICE_ACCOUNT entities (server-side filtered)
// to focus on IAM-relevant security risks rather than infrastructure issues.
func (c *client) ListIssues(ctx context.Context, cursor *string) (*IssueConnection, error) {
	query := `
		query ListIssues($cursor: String) {
			issues(first: 100, after: $cursor, filterBy: {
				status: [OPEN, IN_PROGRESS],
				relatedEntity: {
					type: [USER_ACCOUNT, SERVICE_ACCOUNT]
				}
			}) {
				nodes {
					id
					type
					severity
					status
					createdAt
					sourceRule {
						name
					}
					entitySnapshot {
						id
						externalId
						cloudPlatform
						type
						name
					}
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}
	`

	variables := map[string]interface{}{}
	if cursor != nil && *cursor != "" {
		variables["cursor"] = *cursor
	}

	var result issuesQueryResponse
	if err := c.graphQLRequest(ctx, query, variables, &result); err != nil {
		return nil, fmt.Errorf("failed to list issues: %w", err)
	}

	return &result.Issues, nil
}

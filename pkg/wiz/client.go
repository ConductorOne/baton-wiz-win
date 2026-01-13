package wiz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
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
	httpClient *http.Client
	apiURL     string
}

// NewClient creates a new Wiz API client with OAuth2 authentication.
func NewClient(ctx context.Context, apiURL, clientID, clientSecret, authEndpoint string) (Client, error) {
	if apiURL == "" || clientID == "" || clientSecret == "" || authEndpoint == "" {
		return nil, fmt.Errorf("all authentication parameters are required")
	}

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

	return &client{
		httpClient: httpClient,
		apiURL:     apiURL,
	}, nil
}

// graphQLRequest makes a GraphQL request to the Wiz API with retry logic for rate limits.
func (c *client) graphQLRequest(ctx context.Context, query string, variables map[string]interface{}, result interface{}) error {
	const (
		maxRetries     = 5
		baseDelay      = 1 * time.Second
		maxDelay       = 32 * time.Second
		jitterFraction = 0.1
	)

	requestBody := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Create a new request for each attempt
		req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL, bytes.NewBuffer(jsonBody))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to execute request: %w", err)
			// Network errors should be retried
			if attempt < maxRetries {
				time.Sleep(calculateBackoff(attempt, baseDelay, maxDelay, jitterFraction))
				continue
			}
			return lastErr
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		// Handle rate limiting with retry
		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("rate limited (429)")
			if attempt < maxRetries {
				delay := calculateBackoff(attempt, baseDelay, maxDelay, jitterFraction)
				time.Sleep(delay)
				continue
			}
			return fmt.Errorf("rate limit exceeded after %d retries: %s", maxRetries, string(body))
		}

		// Handle other non-200 status codes
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
		}

		// Parse the response
		var gqlResp graphQLResponse
		gqlResp.Data = result

		if err := json.Unmarshal(body, &gqlResp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		if len(gqlResp.Errors) > 0 {
			return fmt.Errorf("graphql errors: %+v", gqlResp.Errors)
		}

		return nil
	}

	return lastErr
}

// calculateBackoff computes exponential backoff with jitter.
func calculateBackoff(attempt int, baseDelay, maxDelay time.Duration, jitterFraction float64) time.Duration {
	// Calculate exponential backoff: baseDelay * 2^attempt
	backoff := float64(baseDelay) * math.Pow(2, float64(attempt))
	
	// Cap at maxDelay
	if backoff > float64(maxDelay) {
		backoff = float64(maxDelay)
	}
	
	// Add jitter: random value between [backoff * (1-jitterFraction), backoff * (1+jitterFraction)]
	jitter := backoff * jitterFraction * (2*rand.Float64() - 1)
	backoff += jitter
	
	return time.Duration(backoff)
}

// ListUsers retrieves a paginated list of users from Wiz.
// Note: Uses userAccounts instead of users because service accounts with OAuth2
// credentials don't have permission to query the users endpoint.
func (c *client) ListUsers(ctx context.Context, cursor *string) (*UserConnection, error) {
	query := `
		query ListUsers($first: Int, $after: String) {
			userAccounts(first: $first, after: $after) {
				nodes {
					id
					name
					email
				}
				pageInfo {
					endCursor
					hasNextPage
				}
				totalCount
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
		UserAccounts UserConnection `json:"userAccounts"`
	}
	if err := c.graphQLRequest(ctx, query, variables, &result); err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	return &result.UserAccounts, nil
}

// ListProjects retrieves a paginated list of projects from Wiz.
func (c *client) ListProjects(ctx context.Context, cursor *string) (*ProjectConnection, error) {
	query := `
		query ListProjects($cursor: String) {
			projects(first: 100, after: $cursor) {
				edges {
					node {
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
	connection := &UserRoleConnection{
		Edges: make([]UserRoleEdge, len(result.UserRolesV2)),
	}
	for i, role := range result.UserRolesV2 {
		connection.Edges[i] = UserRoleEdge{
			Node: role,
		}
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
				edges {
					node {
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


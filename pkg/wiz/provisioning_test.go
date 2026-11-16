package wiz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type gqlRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

// newTestClient starts a server that issues tokens on /token and answers
// GraphQL on /graphql with handler, and returns a client pointed at it.
func newTestClient(t *testing.T, handler func(w http.ResponseWriter, req gqlRequest)) Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		var req gqlRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		w.Header().Set("Content-Type", "application/json")
		handler(w, req)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := NewClient(context.Background(), srv.URL+"/graphql", "id", "secret", srv.URL+"/token")
	require.NoError(t, err)
	return c
}

func TestGetUserTreatsGraphQLNotFoundAsMissing(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ gqlRequest) {
		_, _ = w.Write([]byte(`{"data":{"user":null},"errors":[{"message":"user not found","extensions":{"code":"NOT_FOUND"}}]}`))
	})
	user, err := c.GetUser(context.Background(), "missing")
	require.NoError(t, err)
	require.Nil(t, user)
}

func TestGetUserNullDataIsMissing(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ gqlRequest) {
		_, _ = w.Write([]byte(`{"data":{"user":null}}`))
	})
	user, err := c.GetUser(context.Background(), "missing")
	require.NoError(t, err)
	require.Nil(t, user)
}

func TestGetUserTransport404IsAnError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ gqlRequest) {
		w.WriteHeader(http.StatusNotFound)
	})
	_, err := c.GetUser(context.Background(), "u1")
	require.Error(t, err, "a wrong API URL must not read as a missing user")
}

func TestGraphQLErrorsAreClassified(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ gqlRequest) {
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"missing scope","extensions":{"code":"FORBIDDEN"}}]}`))
	})
	err := c.UpdateUserRole(context.Background(), "u1", "GLOBAL_ADMIN")
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestGraphQLTransientErrorsAreRetryable(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ gqlRequest) {
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"slow down","extensions":{"code":"RATE_LIMIT_EXCEEDED"}}]}`))
	})
	err := c.UpdateUserRole(context.Background(), "u1", "GLOBAL_ADMIN")
	require.Equal(t, codes.Unavailable, status.Code(err))
}

func TestFindUserByEmailMatchesExactlyAcrossPages(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, req gqlRequest) {
		calls++
		filter, _ := req.Variables["filterBy"].(map[string]interface{})
		require.Equal(t, "a@example.com", filter["search"])
		if req.Variables["after"] == nil {
			_, _ = w.Write([]byte(`{"data":{"users":{"nodes":[{"id":"u9","email":"aa@example.com"}],"pageInfo":{"hasNextPage":true,"endCursor":"c1"}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"users":{"nodes":[{"id":"u1","email":"A@example.com"}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`))
	})
	user, err := c.FindUserByEmail(context.Background(), "a@example.com")
	require.NoError(t, err)
	require.Equal(t, "u1", user.ID)
	require.Equal(t, 2, calls)
}

func TestUpdateUserProjectsSendsExplicitEmptyList(t *testing.T) {
	var got gqlRequest
	c := newTestClient(t, func(w http.ResponseWriter, req gqlRequest) {
		got = req
		_, _ = w.Write([]byte(`{"data":{"updateUser":{"user":{"id":"u1"}}}}`))
	})
	require.NoError(t, c.UpdateUserProjects(context.Background(), "u1", nil))

	input := got.Variables["input"].(map[string]interface{})
	require.Equal(t, "u1", input["id"])
	patch := input["patch"].(map[string]interface{})
	ids, ok := patch["assignedProjectIds"].([]interface{})
	require.True(t, ok, "assignedProjectIds must be sent as a list, not omitted or null")
	require.Empty(t, ids)
}

func TestUpdateProjectMembersPatchesOnlyThatField(t *testing.T) {
	var got gqlRequest
	c := newTestClient(t, func(w http.ResponseWriter, req gqlRequest) {
		got = req
		_, _ = w.Write([]byte(`{"data":{"updateProject":{"project":{"id":"p1"}}}}`))
	})
	require.NoError(t, c.UpdateProjectMembers(context.Background(), "p1", SecurityChampionsField, []string{"u1"}))

	patch := got.Variables["input"].(map[string]interface{})["patch"].(map[string]interface{})
	require.Equal(t, map[string]interface{}{"securityChampions": []interface{}{"u1"}}, patch)
}

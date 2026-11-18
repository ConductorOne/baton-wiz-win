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
		require.Equal(t, "Bearer test", r.Header.Get("Authorization"))
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

func TestGetUserDecodesRoleScope(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, req gqlRequest) {
		require.Equal(t, "u1", req.Variables["id"])
		_, _ = w.Write([]byte(`{"data":{"user":{"id":"u1","email":"a@example.com","effectiveRole":{"id":"PROJECT_READER","isProjectScoped":true},"effectiveAssignedProjects":[{"id":"p1"}]}}}`))
	})
	user, err := c.GetUser(context.Background(), "u1")
	require.NoError(t, err)
	require.True(t, user.EffectiveRole.IsProjectScoped)
	require.Equal(t, "p1", user.EffectiveAssignedProjects[0].ID)
}

func TestGetUserTransport404IsAnError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ gqlRequest) {
		w.WriteHeader(http.StatusNotFound)
	})
	_, err := c.GetUser(context.Background(), "u1")
	require.Error(t, err, "a wrong endpoint URL must not read as a missing user")
}

func TestProvisioningGraphQLErrorsAreClassified(t *testing.T) {
	for code, want := range map[string]codes.Code{
		"NOT_FOUND":           codes.NotFound,
		"FORBIDDEN":           codes.PermissionDenied,
		"BAD_USER_INPUT":      codes.InvalidArgument,
		"RATE_LIMIT_EXCEEDED": codes.Unavailable,
		"SOMETHING_ELSE":      codes.Unknown,
	} {
		t.Run(code, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ gqlRequest) {
				_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"x","extensions":{"code":"` + code + `"}}]}`))
			})
			err := c.UpdateUserRole(context.Background(), "u1", "GLOBAL_ADMIN")
			require.Equal(t, want, status.Code(err))
		})
	}
}

func TestSyncGraphQLErrorsStayUnknown(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ gqlRequest) {
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"x","extensions":{"code":"NOT_FOUND"}}]}`))
	})
	_, err := c.ListUsers(context.Background(), nil)
	require.Equal(t, codes.Unknown, status.Code(err), "sync error handling must be unchanged")
}

func TestNextPageToken(t *testing.T) {
	next, err := NextPageToken(PageInfo{HasNextPage: false, EndCursor: "c1"})
	require.NoError(t, err)
	require.Empty(t, next, "the last page must end pagination")

	next, err = NextPageToken(PageInfo{HasNextPage: true, EndCursor: "c1"})
	require.NoError(t, err)
	require.Equal(t, "c1", next)

	_, err = NextPageToken(PageInfo{HasNextPage: true})
	require.Error(t, err, "ending the listing early would read unlisted objects as deleted")
}

func TestUpdateUserRoleRequestBody(t *testing.T) {
	var got gqlRequest
	c := newTestClient(t, func(w http.ResponseWriter, req gqlRequest) {
		got = req
		_, _ = w.Write([]byte(`{"data":{"updateUser":{"user":{"id":"u1"}}}}`))
	})
	require.NoError(t, c.UpdateUserRole(context.Background(), "u1", "GLOBAL_READER"))

	input := got.Variables["input"].(map[string]interface{})
	require.Equal(t, "u1", input["id"])
	require.Equal(t, map[string]interface{}{"role": "GLOBAL_READER"}, input["patch"])
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

func TestGetProjectNotFoundIsMissing(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ gqlRequest) {
		_, _ = w.Write([]byte(`{"data":{"project":null},"errors":[{"message":"nope","extensions":{"code":"NOT_FOUND"}}]}`))
	})
	p, err := c.GetProject(context.Background(), "p1")
	require.NoError(t, err)
	require.Nil(t, p)
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

func TestFindUserByEmailBrokenPageIsAnError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ gqlRequest) {
		_, _ = w.Write([]byte(`{"data":{"users":{"nodes":[],"pageInfo":{"hasNextPage":true,"endCursor":""}}}}`))
	})
	_, err := c.FindUserByEmail(context.Background(), "a@example.com")
	require.Error(t, err, "a broken page must not read as a missing user")
}

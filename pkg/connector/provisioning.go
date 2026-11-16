package connector

import (
	"context"
	"fmt"
	"strings"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-wiz-win/pkg/wiz"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	memberEntitlement   = "member"
	ownerEntitlement    = "owner"
	championEntitlement = "champion"

	// profileUserIDKey holds the native Wiz user ID. The resource ID is the
	// user's email, so provisioning needs this to address the user in Wiz.
	profileUserIDKey = "user_id"
)

// resolveUser loads the Wiz user behind a principal. It prefers the native ID
// recorded in the profile during sync and falls back to an exact email match,
// which covers principals synced before the ID was recorded.
func resolveUser(ctx context.Context, client wiz.Client, principal *v2.ResourceId, principalResource *v2.Resource) (*wiz.UserDetail, error) {
	if principal.GetResourceType() != UserResourceTypeID {
		return nil, status.Errorf(codes.InvalidArgument, "wiz-connector: only users can be provisioned, got principal type %q", principal.GetResourceType())
	}

	if principalResource != nil {
		if profile := resource.GetProfile(principalResource); profile != nil {
			if v, ok := profile.GetFields()[profileUserIDKey]; ok && v.GetStringValue() != "" {
				user, err := client.GetUser(ctx, v.GetStringValue())
				if err != nil {
					return nil, fmt.Errorf("wiz-connector: failed to get user %q: %w", v.GetStringValue(), err)
				}
				if user != nil {
					return user, nil
				}
			}
		}
	}

	email := principal.GetResource()
	if email == "" {
		return nil, status.Error(codes.InvalidArgument, "wiz-connector: principal has no user ID")
	}
	user, err := client.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("wiz-connector: failed to look up user by email: %w", err)
	}
	if user == nil {
		return nil, status.Errorf(codes.NotFound, "wiz-connector: no Wiz user found with email %q", email)
	}
	return user, nil
}

// entitlementSlug returns the entitlement's slug, deriving it from the ID
// ("<type>:<resource>:<slug>") when the slug is not set.
func entitlementSlug(e *v2.Entitlement) string {
	if s := e.GetSlug(); s != "" {
		return s
	}
	id := e.GetId()
	if i := strings.LastIndex(id, ":"); i >= 0 {
		return id[i+1:]
	}
	return id
}

func projectIDs(refs []wiz.ProjectRef) []string {
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.ID)
	}
	return ids
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func removeString(list []string, v string) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		if s != v {
			out = append(out, s)
		}
	}
	return out
}

// memberIDs returns the member IDs of a project list and whether the user is
// already in it. A member matches on Wiz user ID or, failing that, email.
func memberIDs(members []wiz.ProjectMember, user *wiz.UserDetail) ([]string, bool) {
	ids := make([]string, 0, len(members))
	found := false
	for _, m := range members {
		if m.ID == user.ID || (m.Email != "" && strings.EqualFold(m.Email, user.Email)) {
			found = true
		}
		ids = append(ids, m.ID)
	}
	return ids, found
}

func removeMember(members []wiz.ProjectMember, user *wiz.UserDetail) []string {
	ids := make([]string, 0, len(members))
	for _, m := range members {
		if m.ID == user.ID || (m.Email != "" && strings.EqualFold(m.Email, user.Email)) {
			continue
		}
		ids = append(ids, m.ID)
	}
	return ids
}

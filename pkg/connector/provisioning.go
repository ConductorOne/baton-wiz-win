package connector

import (
	"context"
	"errors"
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

	// profileUserIDKey holds the native Wiz user ID on user resources.
	profileUserIDKey = "user_id"

	profileOwnerEmailsKey    = "owner_emails"
	profileChampionEmailsKey = "champion_emails"
)

// userNotFoundError marks a principal with no Wiz user. Revoke treats it as
// already revoked; matching on this type, not on codes.NotFound, keeps
// transport 404s from reading as a revoked grant.
type userNotFoundError struct {
	userID string
}

func (e *userNotFoundError) Error() string {
	return fmt.Sprintf("wiz-connector: Wiz user %q not found", e.userID)
}

func (e *userNotFoundError) GRPCStatus() *status.Status {
	return status.New(codes.NotFound, e.Error())
}

func isUserNotFound(err error) bool {
	var nf *userNotFoundError
	return errors.As(err, &nf)
}

// resolveWizUser loads the Wiz user behind a principal. The user resource
// ID is the email, kept so existing syncs, grants and policies keep their
// IDs. Provisioning addresses the user by the stable Wiz user ID recorded
// during sync (external ID, then the user_id profile key). Only principals
// synced before that ID was recorded fall back to an exact email match.
func resolveWizUser(ctx context.Context, c wiz.Client, principal *v2.Resource) (*wiz.User, error) {
	if principal.GetId().GetResourceType() != userResourceType.Id {
		return nil, status.Errorf(codes.InvalidArgument, "wiz-connector: only Wiz users can be provisioned, got principal type %q", principal.GetId().GetResourceType())
	}

	if userID := principalWizUserID(principal); userID != "" {
		user, err := c.GetUser(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("wiz-connector: failed to get user %q: %w", userID, err)
		}
		if user == nil {
			return nil, &userNotFoundError{userID: userID}
		}
		return user, nil
	}

	email := principal.GetId().GetResource()
	if email == "" {
		return nil, status.Error(codes.InvalidArgument, "wiz-connector: principal has no user ID or email")
	}
	user, err := c.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("wiz-connector: failed to find user %q: %w", email, err)
	}
	if user == nil {
		return nil, &userNotFoundError{userID: email}
	}
	return user, nil
}

// principalWizUserID returns the Wiz user ID recorded on the principal during
// sync, or "" for a principal synced before it was recorded.
func principalWizUserID(principal *v2.Resource) string {
	if id := principal.GetExternalId().GetId(); id != "" { //nolint:staticcheck // Still set during sync; profile is the fallback.
		return id
	}
	if ids := profileStrings(principal, profileUserIDKey); len(ids) > 0 {
		return ids[0]
	}
	return ""
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

// projectMember is a user listed as a project owner or security champion.
type projectMember struct {
	ID    string
	Email string
}

func isSameMember(m projectMember, user *wiz.User) bool {
	return (m.ID != "" && m.ID == user.ID) || (m.Email != "" && strings.EqualFold(m.Email, user.Email))
}

// otherMemberIDs returns the IDs of the list's members other than the user,
// and whether the user is in the list. The update replaces the whole list, so
// a member without an ID is an error rather than being sent as "" or dropped.
func otherMemberIDs(members []projectMember, user *wiz.User) ([]string, bool, error) {
	ids := make([]string, 0, len(members))
	found := false
	for _, m := range members {
		if isSameMember(m, user) {
			found = true
			continue
		}
		if m.ID == "" {
			return nil, false, status.Errorf(codes.FailedPrecondition, "wiz-connector: project member %q has no user ID", m.Email)
		}
		ids = append(ids, m.ID)
	}
	return ids, found, nil
}

func projectMembers(project *wiz.Project, field wiz.ProjectMemberField) []projectMember {
	var out []projectMember
	if field == wiz.ProjectOwnersField {
		for _, o := range project.ProjectOwners {
			out = append(out, projectMember{ID: o.ID, Email: o.Email})
		}
		return out
	}
	for _, c := range project.SecurityChampions {
		out = append(out, projectMember{ID: c.ID, Email: c.Email})
	}
	return out
}

func profileStrings(r *v2.Resource, key string) []string {
	profile := resource.GetProfile(r)
	if profile == nil {
		return nil
	}
	v, ok := profile.GetFields()[key]
	if !ok {
		return nil
	}
	if s := v.GetStringValue(); s != "" {
		return []string{s}
	}
	var out []string
	for _, item := range v.GetListValue().GetValues() {
		if s := item.GetStringValue(); s != "" {
			out = append(out, s)
		}
	}
	return out
}

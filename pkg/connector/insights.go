package connector

import (
	"context"
	"fmt"
	"strings"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-wiz-win/pkg/wiz"
)

type insightBuilder struct {
	client wiz.Client
}

func (i *insightBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return securityInsightResourceType
}

// detectAppHint attempts to determine the cloud provider from an external resource ID.
func detectAppHint(externalID string) string {
	switch {
	case strings.HasPrefix(externalID, "arn:aws:"):
		return "aws"
	case strings.HasPrefix(externalID, "/subscriptions/"):
		return "azure"
	case strings.HasPrefix(externalID, "//"):
		return "gcp"
	case strings.Contains(externalID, "projects/") && strings.Contains(externalID, "/"):
		return "gcp"
	default:
		return "unknown"
	}
}

// List returns security insights from Wiz as resource objects with SecurityInsightTrait.
// This properly handles pagination by returning one page at a time.
func (i *insightBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, attr resource.SyncOpAttrs) ([]*v2.Resource, *resource.SyncOpResults, error) {
	var insights []*v2.Resource

	// Get the page token from the sync attributes
	var cursor *string
	if attr.PageToken.Token != "" {
		cursor = &attr.PageToken.Token
	}

	// Fetch one page of issues
	resp, err := i.client.ListIssues(ctx, cursor)
	if err != nil {
		return nil, nil, fmt.Errorf("wiz-connector: failed to list issues: %w", err)
	}

	for _, issue := range resp.Nodes {
		// Skip issues without external IDs or issue IDs
		// Note: Server-side filtering already ensures we only get USER_ACCOUNT and SERVICE_ACCOUNT issues
		if issue.EntitySnapshot.ExternalID == "" || issue.ID == "" {
			continue
		}

		// Create a unique resource ID combining issue ID and external resource ID
		resourceID := fmt.Sprintf("%s:%s", issue.ID, issue.EntitySnapshot.ExternalID)

		// Create the insight value with severity and rule name
		insightValue := fmt.Sprintf("[%s] %s: %s", issue.Severity, issue.Type, issue.SourceRule.Name)

		// Detect the app hint from the external ID
		appHint := detectAppHint(issue.EntitySnapshot.ExternalID)

		// Determine cloud platform string for description
		cloudPlatform := "Unknown"
		if issue.EntitySnapshot.CloudPlatform != nil {
			cloudPlatform = *issue.EntitySnapshot.CloudPlatform
		}

		// Create a security insight resource targeting the external resource using the new oneof-based API
		insightResource, err := resource.NewResource(
			fmt.Sprintf("%s - %s", issue.SourceRule.Name, issue.EntitySnapshot.Name),
			securityInsightResourceType,
			resourceID,
			resource.WithSecurityInsightTrait(
				resource.WithIssue(insightValue),
				resource.WithIssueSeverity(issue.Severity),
				resource.WithInsightExternalResourceTarget(issue.EntitySnapshot.ExternalID, appHint),
				resource.WithInsightObservedAt(issue.CreatedAt),
			),
			resource.WithDescription(fmt.Sprintf(
				"Wiz Security Issue: %s (Status: %s, Severity: %s) affecting %s resource %s",
				issue.SourceRule.Name,
				issue.Status,
				issue.Severity,
				cloudPlatform,
				issue.EntitySnapshot.Name,
			)),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("wiz-connector: failed to create security insight resource: %w", err)
		}

		insights = append(insights, insightResource)
	}

	// Prepare the sync results with next page token if there are more pages
	syncResults := &resource.SyncOpResults{}
	if resp.PageInfo.HasNextPage {
		syncResults.NextPageToken = resp.PageInfo.EndCursor
	}

	return insights, syncResults, nil
}

// Entitlements returns an empty slice as security insights are informational resources.
func (i *insightBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ resource.SyncOpAttrs) ([]*v2.Entitlement, *resource.SyncOpResults, error) {
	return nil, nil, nil
}

// Grants returns an empty slice as security insights don't have grants.
func (i *insightBuilder) Grants(ctx context.Context, resource *v2.Resource, attr resource.SyncOpAttrs) ([]*v2.Grant, *resource.SyncOpResults, error) {
	return nil, nil, nil
}

func newInsightBuilder(client wiz.Client) *insightBuilder {
	return &insightBuilder{client: client}
}

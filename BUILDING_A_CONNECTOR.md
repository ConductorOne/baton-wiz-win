# Building a Baton Connector

This guide explains how to build a connector using the Baton template.

## Understanding the Purpose of Baton Connectors

Baton connectors serve a specific purpose: to help companies identify **who has access to what resources** in the services they use. This information is critical for compliance with SOC2 auditing requirements and maintaining proper access controls.

### Focus on Access Control, Not Service Data

When building a connector, focus exclusively on resources related to access control, permissions, and administrative capabilities - not the operational data within the service.

#### Examples:

* **Salesforce**: Sync users with licenses, roles, permissions, and groups - not customer data, contacts, or sales opportunities
* **Greenhouse**: Sync users who can administer Greenhouse - not applicant data or hiring processes
* **Jira**: Sync users with admin roles and project access permissions - not issues, tickets, or project metrics

### Good Resources to Sync

Focus on resources that answer these questions:
- Who can administer the service?
- Who has owner or elevated privileges?
- Who can modify critical configurations?
- Who has read access to sensitive information?

## Prerequisites

- Go 1.25.x (should match the latest version in https://github.com/ConductorOne/baton-sdk)
- Access to the system you want to connect to

## Getting Started

1. Go to the private [baton-template repository](https://github.com/ConductorOne/baton-template) (requires access).

2. Click the "Use this template" button to create a new repository in your account with the correct structure.

3. Name your repository following the `baton-service-name` pattern (e.g., `baton-microsoft-entra`, `baton-okta`, `baton-retool`, `baton-google-cloud-platform`).

4. After creating the repository, clone it to your local machine.

5. Edit the `cookiecutter.json` file as instructed in the README.md with your repository information:
   ```json
   {
     "repo_owner": "your-github-username",
     "repo_name": "baton-service-name",
     "name": "baton-service-name",
     "config_name": "service-name"
   }
   ```

6. Commit and push this change. A GitHub action will run automatically to generate a basic connector with a user resource builder that you can use as a starting point.

## Implementing Connector Logic

The main connector logic should be implemented in the `pkg/connector` directory:

- `connector.go`: Contains the main connector implementation
- `resource_types.go`: Define resource types for your connector
- `users.go`: Implement user-related functionality

Each resource type should have its own Go file that implements a resource syncer's required methods:

```go
func (o *userResourceType) ResourceType(ctx context.Context) *v2.ResourceType

func (o *userResourceType) List(
	ctx context.Context,
	resourceID *v2.ResourceId,
	token *pagination.Token,
) ([]*v2.Resource, string, annotations.Annotations, error) 

func (o *userResourceType) Entitlements(
	_ context.Context,
	resource *v2.Resource,
	_ *pagination.Token,
) ([]*v2.Entitlement, string, annotations.Annotations, error) 

func (o *userResourceType) Grants(
	ctx context.Context,
	resource *v2.Resource,
	token *pagination.Token,
) ([]*v2.Grant, string, annotations.Annotations, error) 
```

Optionally, if provisioning is supported, implement Grant and Revoke methods:

```go
func (g *groupResourceType) Grant(ctx context.Context, principal *v2.Resource, entitlement *v2.Entitlement) (annotations.Annotations, error) 

func (g *groupResourceType) Revoke(ctx context.Context, grant *v2.Grant) (annotations.Annotations, error) 
```

## Core Components to Implement

1. **Resource Types**: Define the access-related resources your connector will sync
2. **Syncing Logic**: Implement how to fetch and sync access control resources from the target system
3. **Authentication**: Implement how your connector authenticates with the target system

### Defining Resource Types

After generating your connector files using the template, you need to define resource types in your `pkg/connector/resource_types.go` file.

The Baton SDK provides well-known resource traits to categorize common resource types:
- `TRAIT_USER`: For user resources
- `TRAIT_GROUP`: For group resources
- `TRAIT_ROLE`: For role resources
- `TRAIT_APP`: For application resources

Example resource type definitions:

```go
package connector

import v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"

var (
    userResourceType = &v2.ResourceType{
        Id:          "user",
        DisplayName: "User",
        Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_USER},
    }
    resourceTypeRole = &v2.ResourceType{
        Id:          "role",
        DisplayName: "Role",
        Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_ROLE},
    }
    resourceTypeGroup = &v2.ResourceType{
        Id:          "group",
        DisplayName: "Group",
        Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_GROUP},
    }
)
```

Each resource type should:
1. Have a unique `Id` within your connector
2. Have a descriptive `DisplayName` that will appear in the UI
3. Include the appropriate `Traits` to indicate what type of resource it is

### Implementing the Connector

Your connector will need to implement several key interfaces from the Baton SDK. This is typically done across several files in the `pkg/connector` directory.

#### 1. Setting Up Client Access

There are multiple approaches to access the target service:

**Option A: Using the service's official SDK (preferred if available)**
```go
// pkg/connector/client.go
package connector

import (
    "context"
    
    "github.com/example-service/go-sdk" // Target service's official SDK
)

type Client struct {
    client *sdk.Client
}

func NewClient(ctx context.Context, apiKey string) (*Client, error) {
    sdkClient, err := sdk.NewClient(apiKey)
    if err != nil {
        return nil, err
    }
    
    return &Client{
        client: sdkClient,
    }, nil
}
```

**Option B: Using uhttp client (when SDK isn't available)**
```go
// pkg/connector/client.go
package connector

import (
    "context"
    "net/http"
    "net/url"
    
    "github.com/conductorone/baton-sdk/pkg/uhttp"
)

type Client struct {
    httpClient *uhttp.BaseHttpClient
    baseURL    string
    apiKey     string
}

func NewClient(ctx context.Context, apiKey string) (*Client, error) {
    // Preferred approach: using NewBaseHttpClientWithContext
    httpClient, err := uhttp.NewBaseHttpClientWithContext(ctx, http.DefaultClient)
    if err != nil {
        return nil, err
    }
    
    // This is preferred over using the regular http.Client directly
    // as it provides automatic rate limiting handling, error wrapping with gRPC status codes,
    // and built-in GET response caching
    
    return &Client{
        httpClient: httpClient,
        baseURL:    "https://api.yourservice.com/v1",
        apiKey:     apiKey,
    }, nil
}

// Example API request using uhttp BaseHttpClient
func (c *Client) GetUsers(ctx context.Context) ([]User, error) {
    // Create the URL
    parsedURL, err := url.Parse(c.baseURL + "/users")
    if err != nil {
        return nil, err
    }
    
    // Create the request with BaseHttpClient's helper
    req, err := c.httpClient.NewRequest(
        ctx,
        "GET",
        parsedURL,
        uhttp.WithHeader("Authorization", "Bearer " + c.apiKey),
        uhttp.WithAcceptJSONHeader(),
    )
    if err != nil {
        return nil, err
    }
    
    // Define the response structure
    var response struct {
        Users []User `json:"users"`
    }
    
    // Do the request and handle the response
    resp, err := c.httpClient.Do(req, uhttp.WithJSONResponse(&response))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    return response.Users, nil
}
```

#### 2. Implementing Resource Syncing

Each resource type needs corresponding implementation to sync data. User resources require special attention as they need specific information like status, email, and profile data.

##### User Resources

When implementing user resources, it's important to include:

- User status (enabled/disabled)
- Email address(es)
- User profile information
- Account type
- Creation and last login timestamps

Here's how to implement user resource syncing:

```go
// pkg/connector/users.go
func (u *userResourceType) List(ctx context.Context, parentResourceID *v2.ResourceId, token *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
    // Parse pagination token if needed
    
    // Fetch users from your service API with pagination
    users, nextPage, err := u.client.GetUsers(ctx, token)
    if err != nil {
        return nil, "", nil, err
    }
    
    var resources []*v2.Resource
    for _, user := range users {
        // Determine user status
        userStatus := v2.UserTrait_Status_STATUS_ENABLED
        if !user.Active {
            userStatus = v2.UserTrait_Status_STATUS_DISABLED
        }
        
        // Set up user trait options
        userTraitOptions := []sdkResource.UserTraitOption{
            sdkResource.WithEmail(user.Email, true),
            sdkResource.WithStatus(userStatus),
            sdkResource.WithUserProfile(map[string]interface{}{
                "login": user.Login,
                "first_name": user.FirstName,
                "last_name": user.LastName,
                "user_id": user.ID,
                // Include any other relevant user profile data
            }),
        }
        
        // Add optional information if available
        if user.CreatedAt != nil {
            userTraitOptions = append(userTraitOptions, sdkResource.WithCreatedAt(*user.CreatedAt))
        }
        if user.LastLogin != nil {
            userTraitOptions = append(userTraitOptions, sdkResource.WithLastLogin(*user.LastLogin))
        }
        
        // Create the user resource using the SDK helper
        userResource, err := sdkResource.NewUserResource(
            user.DisplayName,
            userResourceType,
            user.ID,
            userTraitOptions,
            sdkResource.WithParentResourceID(parentResourceID),
        )
        if err != nil {
            return nil, "", nil, err
        }
        
        resources = append(resources, userResource)
    }
    
    return resources, nextPage, nil, nil
}
```

#### 3. Implementing Configuration and Main Entry Point

Every Baton connector needs a configuration schema and a main entry point. The template automatically generates the main.go file, but you'll need to implement the configuration schema and the connector initialization. Note that configuration, field names, and field settings must match exactly with what's already defined in C1's builtin connector schema. This is important in order to maintain consistency and avoid mapping issues.
##### Configuration Schema

Create or update a config.go file to define the command-line arguments that your connector accepts:

```go
// pkg/config/config.go
package config

import (
    "github.com/conductorone/baton-sdk/pkg/field"
)

var (
    // Define your configuration fields
    APIKeyField = field.StringField(
        "api-key",
        field.WithDescription("API key for authenticating with the service"),
        field.WithRequired(true),
    )
    
    // Optional fields example
    BaseURLField = field.StringField(
        "base-url",
        field.WithDescription("Base URL for the service API (optional, defaults to https://api.example.com)"),
    )
    
    // Boolean flag example
    IncludeDisabledUsersField = field.BoolField(
        "include-disabled-users",
        field.WithDescription("Include disabled users in the sync (defaults to false)"),
    )
    
    // Configuration combines all fields
    Configuration = field.NewConfiguration([]field.SchemaField{
        APIKeyField,
        BaseURLField,
        IncludeDisabledUsersField,
    })
)
```

##### Main Entry Point

Implement the main.go file to use the configuration and initialize your connector:

```go
// cmd/baton-service-name/main.go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/your-repo/baton-service-name/pkg/connector"
    "github.com/your-repo/baton-service-name/pkg/config"
    
    "github.com/conductorone/baton-sdk/pkg/connectorbuilder"
    "github.com/conductorone/baton-sdk/pkg/types"
    "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
    "github.com/spf13/viper"
    "go.uber.org/zap"
)

var (
    connectorName = "baton-service-name"
    version       = "dev"
)

func main() {
    ctx := context.Background()

    // Define the command configuration using baton-sdk
    _, cmd, err := config.DefineConfiguration(
        ctx,
        connectorName,
        getConnector,
        config.Configuration,
    )
    if err != nil {
        fmt.Fprintln(os.Stderr, err.Error())
        os.Exit(1)
    }

    cmd.Version = version

    // Execute the command
    err = cmd.Execute()
    if err != nil {
        fmt.Fprintln(os.Stderr, err.Error())
        os.Exit(1)
    }
}

// getConnector initializes and returns your connector
func getConnector(ctx context.Context, cfg *config.ConnectorConfigName) (types.ConnectorServer, error) {
    l := ctxzap.Extract(ctx)
    
    // Extract configuration values using field names
    apiKey := cfg.APIKeyField
    baseURL := cfg.BaseURLField
    includeDisabledUsers := cfg.IncludeDisabledUsersField
    
    // Create your connector with the configuration
    cb, err := connector.New(
        ctx,
        apiKey,
        baseURL,
        includeDisabledUsers,
    )
    if err != nil {
        l.Error("error creating connector", zap.Error(err))
        return nil, err
    }

    // Wrap your connector with the SDK's ConnectorServer
    c, err := connectorbuilder.NewConnector(ctx, cb)
    if err != nil {
        l.Error("error creating connector server", zap.Error(err))
        return nil, err
    }

    return c, nil
}
```

This pattern has several benefits:
- It automatically handles command-line argument parsing using Viper and Cobra (handled by the SDK)
- It provides proper validation of required fields
- It integrates with the Baton SDK's logging and error handling
- It creates a standard CLI interface consistent with other Baton connectors

#### Implementing Thread-Safe Caching

When building connectors, you may need to make the same API requests multiple times. For example, when implementing a `Grants` method for roles, you might need to check if each user has a specific role, but the API doesn't provide a way to filter by role directly.

To avoid making redundant API calls, you can implement a thread-safe cache using a mutex:

```go
type resourceBuilder struct {
    client *Client
    
    // Cache for API responses
    cachedData []SomeDataType
    cacheMutex sync.Mutex
}

// Thread-safe caching method
func (r *resourceBuilder) getCachedData(ctx context.Context) ([]SomeDataType, error) {
    // Lock the mutex to ensure thread safety
    r.cacheMutex.Lock()
    defer r.cacheMutex.Unlock()
    
    // Return cached data if it exists
    if r.cachedData != nil {
        return r.cachedData, nil
    }
    
    // Otherwise, fetch the data from the API
    data, err := r.client.FetchData(ctx)
    if err != nil {
        return nil, err
    }
    
    // Store in cache for future use
    r.cachedData = data
    
    return data, nil
}

// Use the cached data in your Grants method
func (r *resourceBuilder) Grants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
    // Get the data using the thread-safe cache
    data, err := r.getCachedData(ctx)
    if err != nil {
        return nil, "", nil, err
    }
    
    var grants []*v2.Grant
    
    // Process the cached data
    for _, item := range data {
        if itemHasPermission(item, resource.Id.Resource) {
            // Create grant...
            grants = append(grants, grant)
        }
    }
    
    return grants, "", nil, nil
}
```

This caching pattern is especially useful when:
- The API doesn't provide efficient filtering or pagination options
- You need to make the same expensive API call for multiple resources
- You're working with nested resources that require the same parent data

**Important caching considerations:**

1. In-memory caches are transient and will be lost if:
   - The connector is restarted during a sync operation
   - The process is terminated unexpectedly
   - The sync takes a very long time (hours) and encounters interruptions

2. The Baton SDK uses checkpointing to resume syncs where they left off, but any in-memory cache will be reset when the process restarts

3. Design your cache implementation to handle cache misses gracefully:
   - Always check if data exists in the cache
   - Be prepared to refetch data if needed
   - Consider using pagination tokens to avoid having to refetch all data

#### 4. Implementing Entitlements and Grants

Entitlements represent what permissions exist, and grants represent who has those permissions.

##### Grant Expansion

In many systems, access permissions can be nested or inherited. For example, a user might be a member of a group, and that group might have access to specific resources. Baton SDK provides a mechanism called "grant expansion" to resolve these nested permissions and determine the complete set of resources a user has access to.

To implement grant expansion:

1. When creating a grant for a resource that itself has access to other resources (like a group), add the `GrantExpandable` annotation
2. Use pagination to handle potentially large sets of nested permissions
3. Implement the Grants method to traverse the hierarchy of permissions

Here's a conceptual implementation for grant expansion:

```go
// When creating a grant for a group that has its own members
grantOptions := []grant.GrantOption{
    grant.WithAnnotation(&v2.GrantExpandable{
        EntitlementIds: []string{"group:member"}, // Reference to the group's membership entitlement
        Shallow: true, // Set to true to only expand one level
    }),
}

// Create the grant with the expandable annotation
groupGrant := grant.NewGrant(
    resource,            // The resource granting access
    "admin",             // The role or permission being granted
    &v2.ResourceId{      // The recipient of the grant (in this case a group)
        ResourceType: resourceTypeGroup.Id,
        Resource:     "engineering-team@example.com",
    },
    grantOptions...,     // Include the expandable annotation
)
```

When implementing the Grants method for a resource type that needs to handle expansion:

```go
func (r *resourceType) Grants(ctx context.Context, resource *v2.Resource, pt *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
    // Set up pagination
    bag := &pagination.Bag{}
    err := bag.Unmarshal(pt.Token)
    if err != nil {
        return nil, "", nil, err
    }
    
    var grants []*v2.Grant
    
    // Initialize pagination state if needed
    if bag.Current() == nil {
        bag.Push(pagination.PageState{
            ResourceTypeID: resource.Id.ResourceType,
        })
    }
    
    // Handle different resource types in the pagination stack
    switch bag.ResourceTypeID() {
    case resourceTypeProject.Id:
        // Handle direct grants to the project
        // ...
        
    case resourceTypeGroup.Id:
        // This section handles expanding group membership
        
        // Get the group members with pagination
        members, nextPageToken, err := getGroupMembers(ctx, bag.ResourceID(), bag.PageToken())
        if err != nil {
            return nil, "", nil, err
        }
        
        // Update pagination state
        err = bag.Next(nextPageToken)
        if err != nil {
            return nil, "", nil, err
        }
        
        // Create grants for each member
        for _, member := range members {
            switch member.Type {
            case "USER":
                // Create direct grant to the user
                grants = append(grants, grant.NewGrant(
                    resource, 
                    "member", 
                    &v2.ResourceId{
                        ResourceType: resourceTypeUser.Id,
                        Resource:     member.Email,
                    },
                ))
                
            case "GROUP":
                // For nested groups, add another expandable annotation
                grantOptions := []grant.GrantOption{
                    grant.WithAnnotation(&v2.GrantExpandable{
                        EntitlementIds: []string{"group:member"},
                        Shallow:        true,
                    }),
                }
                
                grants = append(grants, grant.NewGrant(
                    resource, 
                    "member", 
                    &v2.ResourceId{
                        ResourceType: resourceTypeGroup.Id,
                        Resource:     member.Email,
                    },
                    grantOptions...,
                ))
            }
        }
    }
    
    // Marshal the pagination state for the next page
    nextPageToken, err := bag.Marshal()
    if err != nil {
        return nil, "", nil, err
    }
    
    return grants, nextPageToken, nil, nil
}
```

This grant expansion mechanism allows the Baton SDK to resolve complex permission hierarchies and accurately report who has access to what resources, even when that access is granted indirectly through group memberships or other nested structures.

##### Entitlements Examples

```go
// For discovering which entitlements exist for a resource type
// This method is typically implemented in each resource type builder

// Example for a role resource type
func (r *roleResourceBuilder) Entitlements(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
    var ret []*v2.Entitlement

    // Create a permission entitlement for the role
    ret = append(ret, sdkEntitlement.NewPermissionEntitlement(
        resource,
        "member",
        sdkEntitlement.WithDescription(fmt.Sprintf("Assigned to %s", resource.DisplayName)),
    ))

    return ret, "", nil, nil
}

// Example for a group resource type
func (g *groupResourceType) Entitlements(ctx context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
    var rv []*v2.Entitlement

    // Define options for the entitlement
    assignmentOptions := []ent.EntitlementOption{
        ent.WithGrantableTo(resourceTypeUser),
        ent.WithDescription(fmt.Sprintf("Member of %s group", resource.DisplayName)),
        ent.WithDisplayName(fmt.Sprintf("%s group %s", resource.DisplayName, memberEntitlement)),
    }

    // Create an assignment entitlement
    en := ent.NewAssignmentEntitlement(resource, memberEntitlement, assignmentOptions...)
    rv = append(rv, en)

    return rv, "", nil, nil
}

// Example for a custom role resource type
func (o *customRoleResourceType) Entitlements(
    ctx context.Context,
    resource *v2.Resource,
    token *pagination.Token,
) ([]*v2.Entitlement, string, annotations.Annotations, error) {
    var rv []*v2.Entitlement

    // Create an assignment entitlement with multiple options
    en := sdkEntitlement.NewAssignmentEntitlement(resource, "assigned",
        sdkEntitlement.WithDisplayName(fmt.Sprintf("%s Role Member", resource.DisplayName)),
        sdkEntitlement.WithDescription(fmt.Sprintf("Has the %s role in the system", resource.DisplayName)),
        sdkEntitlement.WithAnnotation(&v2.V1Identifier{
            Id: "some-identifier",
        }),
        sdkEntitlement.WithGrantableTo(resourceTypeUser, resourceTypeGroup),
    )
    rv = append(rv, en)

    return rv, "", nil, nil
}

// For discovering who has which entitlements
// This method is typically implemented in each resource type builder

// Example for a group resource type
func (g *groupResourceBuilder) Grants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
    // Parse pagination token if needed
    bag := &pagination.Bag{}
    if pToken.Token != "" {
        if err := bag.Unmarshal(pToken.Token); err != nil {
            return nil, "", nil, err
        }
    } else {
        // Initialize pagination state
        bag.Push(pagination.PageState{
            ResourceTypeID: resource.Id.ResourceType,
        })
    }

    // Fetch members of the group from your API
    members, nextPageToken, err := fetchGroupMembers(ctx, resource.Id.Resource, bag.PageToken())
    if err != nil {
        return nil, "", nil, fmt.Errorf("failed to get group members: %w", err)
    }

    // Create grants for each member
    var grants []*v2.Grant
    for _, member := range members {
        // Create a grant connecting the user to the group with the member entitlement
        grant := sdkGrant.NewGrant(
            resource,
            "member",
            &v2.ResourceId{
                ResourceType: resourceTypeUser.Id,
                Resource:     member.ID,
            },
        )
        grants = append(grants, grant)
    }

    // Create next page token
    nextToken, err := bag.NextToken(nextPageToken)
    if err != nil {
        return nil, "", nil, err
    }

    return grants, nextToken, nil, nil
}

// Example helper function for fetching group members
func fetchGroupMembers(ctx context.Context, groupID string, pageToken string) ([]User, string, error) {
    // Implementation would make API calls to fetch members
    // This is just a placeholder
    return []User{}, "", nil
}
```

#### 4. Register Resource Types

Register all resource types with the connector:

```go
func (c *Connector) ResourceTypes(ctx context.Context, request *v2.ResourceTypesRequest) (*v2.ResourceTypesResponse, error) {
    return &v2.ResourceTypesResponse{
        ResourceTypes: []*v2.ResourceType{
            userResourceType,
            resourceTypeRole,
            resourceTypeGroup,
            // Add other resource types
        },
    }, nil
}
```

## Running Your Connector

### Building

Build your connector:
```bash
make build
```

### Running a Sync

Run your connector locally with the required authentication parameters for your specific connector. For example, running a Microsoft Entra connector might look like this:

```bash
./dist/darwin_arm64/baton-microsoft-entra \
  --entra-client-id YOUR_CLIENT_ID \
  --entra-client-secret YOUR_CLIENT_SECRET \
  --entra-tenant-id YOUR_TENANT_ID \
  --log-level debug
```

The required parameters will vary depending on which service you're connecting to. Be sure to include all required authentication parameters for your specific connector.

Adding `--log-level debug` is optional but useful for seeing detailed information about what's happening during the sync process.

This will generate a `sync.c1z` file in the current directory containing all the sync data. You don't need to specify an output directory as the connector automatically creates this file.

### Analyzing Sync Results

Use the `baton` CLI tool to examine the sync data and verify it meets your requirements:

```bash
# View all resources of a specific type with JSON output
baton resources -t user -o json | jq

# View all entitlements 
baton entitlements

# View all grants
baton grants

# Other useful commands
baton resource-types     # View all resource types
baton stats              # Show statistics about the sync file
baton syncs              # List information about syncs in the file
baton diff               # Compare differences between sync runs
```

Example output from viewing user resources:
```json
{
  "resources": [
    {
      "resource": {
        "id": {
          "resourceType": "user",
          "resource": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
        },
        "displayName": "example.user",
        "annotations": [
          {
            "@type": "type.googleapis.com/c1.connector.v2.ExternalLink",
            "url": "https://entra.microsoft.com/#view/Microsoft_AAD_UsersAndTenants/UserProfileMenuBlade/~/overview/userId/a1b2c3d4-e5f6-7890-abcd-ef1234567890"
          },
          {
            "@type": "type.googleapis.com/c1.connector.v2.UserTrait",
            "emails": [
              {
                "address": "example.user@example.onmicrosoft.com",
                "isPrimary": true
              }
            ],
            "status": {
              "status": "STATUS_ENABLED"
            },
            "profile": {
              "accountEnabled": true,
              "displayName": "example.user",
              "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
              "mail": "example.user@example.onmicrosoft.com",
              "userPrincipalName": "example.user@example.onmicrosoft.com"
            },
            "accountType": "ACCOUNT_TYPE_HUMAN",
            "login": "example.user@example.onmicrosoft.com",
            "lastLogin": "2025-03-06T12:34:56Z"
          }
        ],
        "creationSource": "CREATION_SOURCE_CONNECTOR_LIST_RESOURCES"
      },
      "resourceType": {
        "id": "user",
        "displayName": "User",
        "traits": [
          "TRAIT_USER"
        ],
        "annotations": [
          {
            "@type": "type.googleapis.com/c1.connector.v2.SkipEntitlementsAndGrants"
          }
        ]
      }
    }
  ]
}
```

### Debugging Common Issues

1. **Empty resources**: Verify your API calls are returning data
2. **Missing entitlements**: Check your implementation of the Entitlements method
3. **Missing grants**: Verify your Grants method implementation returns the correct data
4. **Resource ID mismatches**: Ensure consistent resource IDs between resources and grants

## Packaging

When you create a release in the GitHub repository, CI will automatically build and publish Docker images for your connector. There's no need to manually build Docker images for distribution.

## Best Practices

- Follow Go best practices for error handling
- Implement proper logging
- Consider rate limiting when making API calls
- Use pagination for large data sets
- Handle authentication token renewal
- **Be selective about which resources to sync**: Only sync resources relevant to access control
- **Avoid syncing operational data**: Customer data, business metrics, and other non-access-related information should be excluded

## Properly Selecting Resources

When selecting which resources to sync from your target service:

1. **Start with users**: Always include the service's users as a core resource
2. **Include access control resources**: Roles, permissions, groups, teams, licenses that determine what users can access
3. **Include critical resources**: Resources that require privileged access or contain sensitive information
4. **Exclude operational data**: Sales data, customer records, tickets, tasks, or other business process information

For example, when building a connector for an email service:
- **Include**: User accounts, admin roles, access permissions to shared mailboxes
- **Exclude**: Email contents, contact lists, calendar events

## Authentication Implementation

Most services require authentication to access their APIs. Here are common approaches:

### API Key or Token Authentication

```go
func NewConnector(ctx context.Context, apiKey string) (*Connector, error) {
    client, err := NewClient(ctx, apiKey)
    if err != nil {
        return nil, err
    }
    
    return &Connector{
        client: client,
    }, nil
}
```

### OAuth Authentication

For services using OAuth, prefer client credentials grant with Go's TokenSource:

```go
import (
    "context"
    "golang.org/x/oauth2"
    "golang.org/x/oauth2/clientcredentials"
)

func NewConnector(ctx context.Context, clientID, clientSecret, tokenURL string) (*Connector, error) {
    // Set up client credentials config
    config := &clientcredentials.Config{
        ClientID:     clientID,
        ClientSecret: clientSecret,
        TokenURL:     tokenURL,
        Scopes:       []string{"required_scope1", "required_scope2"},
    }
    
    // Create token source
    tokenSource := config.TokenSource(ctx)
    
    // Create HTTP client with automatic token handling
    httpClient := oauth2.NewClient(ctx, tokenSource)
    
    // Create your service client using the OAuth HTTP client
    client, err := NewClient(ctx, httpClient)
    if err != nil {
        return nil, err
    }
    
    return &Connector{
        client: client,
    }, nil
}
```

This approach has several advantages:
- Automatically handles token refresh
- Uses Go's standard oauth2 library
- Provides a clean separation of authentication concerns
- Works well with the Baton SDK's HTTP utilities

## Implementing Pagination

### The Importance of Pagination

Proper pagination is **critical** for building effective Baton connectors. Enterprise organizations often have:
- Tens of thousands of users
- Hundreds or thousands of groups
- Complex permission hierarchies
- Large numbers of resources

**Always assume your connector will need to handle large datasets.** A connector that doesn't implement pagination properly will:
- Fail in production environments with large user bases
- Return incomplete data
- Cause timeouts and resource exhaustion
- Miss critical access control information

While some resource types might have a small number of items (like admin roles), most resource types require pagination. Even if you're testing with a small dataset, implement pagination from the beginning to ensure your connector will work in large enterprise environments.

### Pagination Patterns

Most APIs provide pagination mechanisms. Here's a pattern for handling API pagination:

```go
func (r *resourceBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, pt *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
    // Parse the pagination token if it exists
    bag := &pagination.Bag{}
    err := bag.Unmarshal(pt.Token)
    if err != nil {
        return nil, "", nil, err
    }
    
    // Get the page token from our pagination bag
    pageToken := bag.PageToken()
    pageSize := 100 // Adjust based on API limits and performance
    
    // Fetch a page of data from the API
    data, nextPageToken, err := r.client.GetData(ctx, pageSize, pageToken)
    if err != nil {
        return nil, "", nil, err
    }
    
    // Create resources from the data
    var resources []*v2.Resource
    for _, item := range data {
        resource, err := createResourceFromItem(item)
        if err != nil {
            return nil, "", nil, err
        }
        resources = append(resources, resource)
    }
    
    // Create the next pagination token
    err = bag.Next(nextPageToken)
    if err != nil {
        return nil, "", nil, err
    }
    
    // Create the token for the next page
    nextToken, err := bag.Marshal()
    if err != nil {
        return nil, "", nil, err
    }
    
    return resources, nextToken, nil, nil
}
```

### Handling APIs Without Pagination

Some APIs don't provide pagination mechanisms, especially for resource types that are typically small. In these cases:

1. **Try to fetch data efficiently**: 
   - Use filtering if available to reduce result size
   - Batch requests when possible

2. **Document the limitation**:
   - Note in your code that the API doesn't support pagination
   - Document any size limitations for this resource type

3. **Consider artificial pagination**:
   - If you must fetch all data at once, you can implement artificial pagination in your connector
   - Split large results into chunks and use the pagination bag to track which chunk you're processing


## Rate Limiting

The Baton SDK automatically handles rate limiting through annotations. Instead of implementing manual delays, use the SDK's rate limiting annotations to properly handle API rate limits:

```go
// Example from baton-github showing how to extract rate limit data from API response headers
func extractRateLimitData(response *github.Response) (*v2.RateLimitDescription, error) {
	if response == nil {
		return nil, fmt.Errorf("github-connector: passed nil response")
	}
	var err error

	var r int64
	remaining := response.Header.Get("X-Ratelimit-Remaining")
	if remaining != "" {
		r, err = strconv.ParseInt(remaining, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ratelimit-remaining: %w", err)
		}
	}

	var l int64
	limit := response.Header.Get("X-Ratelimit-Limit")
	if limit != "" {
		l, err = strconv.ParseInt(limit, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ratelimit-limit: %w", err)
		}
	}

	var ra *timestamppb.Timestamp
	resetAt := response.Header.Get("X-Ratelimit-Reset")
	if resetAt != "" {
		ts, err := strconv.ParseInt(resetAt, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ratelimit-reset: %w", err)
		}
		ra = &timestamppb.Timestamp{Seconds: ts}
	}

	return &v2.RateLimitDescription{
		Limit:     l,
		Remaining: r,
		ResetAt:   ra,
	}, nil
}

// Then use the annotations with rate limiting data in your connector methods
func parseAPIResponse(resp *APIResponse) (string, annotations.Annotations, error) {
    var annos annotations.Annotations
    
    // Extract rate limit data and add to annotations
    if desc, err := extractRateLimitData(resp); err == nil {
        annos.WithRateLimiting(desc)
    }
    
    return nextPageToken, annos, nil
}
```

The SDK will automatically handle backing off requests when rate limits are hit, so you don't need to implement delay logic in your connector.

## Documentation

Be sure to update the README.md with specific details about your connector, including:
- Required permissions/scopes to access the API
- Configuration options and environment variables
- Known limitations or API restrictions
- Which resources are synced and why they were selected
- Screenshots of what the connector looks like in the ConductorOne UI
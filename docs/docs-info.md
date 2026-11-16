While developing the connector, please fill out this form. This information is needed to write docs and to help other users set up the connector.

## Connector capabilities

1. What resources does the connector sync?

   The connector syncs the following resources from Wiz:
   
   * **Users** - User accounts from the `userAccounts` GraphQL endpoint. Includes user email, name, and ID. Note: The `users` endpoint is not accessible with service account authentication.
   
   * **Roles** - User roles from the `userRolesV2` GraphQL endpoint. Includes role name, description, scopes, whether it's built-in, and project-scoped status. Role membership grants come from each user's `effectiveRole` on the `users` endpoint.
   
   * **Projects** - Wiz projects/workspaces from the `projects` GraphQL endpoint. Includes project name, description, project owners, and security champions. Creates grants for project owners and security champions (matched by email).
   
   * **Security Insights** - Wiz security issues from the `issues` GraphQL endpoint, filtered to only include issues affecting `USER_ACCOUNT` or `SERVICE_ACCOUNT` entities (~14% of total issues). Uses the `SecurityInsightTrait` annotation to link Wiz findings to external cloud resources (AWS, Azure, GCP) via their external IDs (e.g., AWS ARNs). This enables ConductorOne's Uplift system to match security findings to IAM resources synced from other connectors (baton-aws, baton-azure, etc.).

2. Can the connector provision any resources? If so, which ones? 

   Yes. The connector provisions role membership (`member` on a role: sets the user's single Wiz role, replacing the previous one; a held role cannot be revoked, only replaced) and project access (`member` adds or removes the project from a project-scoped user's assigned projects; `owner` and `champion` add or remove the user from the project's owners or security champions). It does not create or delete accounts.

## Connector credentials 

1. What credentials or information are needed to set up the connector? (For example, API key, client ID and secret, domain, etc.)

   The connector requires four pieces of information:
   
   * **Wiz API URL** - The GraphQL API endpoint for your Wiz region (e.g., `https://api.us83.app.wiz.io/graphql`)
   * **OAuth2 Client ID** - The service account's client ID
   * **OAuth2 Client Secret** - The service account's client secret
   * **OAuth2 Auth Endpoint** - The token endpoint for authentication (typically `https://auth.app.wiz.io/oauth/token`)

2. For each item in the list above: 

   **How does a user create or look up that credential or info?**
   
   1. Log in to the Wiz portal (https://app.wiz.io)
   2. Navigate to Settings > Service Accounts
   3. Click "Add Service Account"
   4. Provide a name (e.g., "ConductorOne Baton Connector")
   5. Select the required permissions (see below)
   6. Click "Create"
   7. Copy the **Client ID** and **Client Secret** (the secret is only shown once!)
   8. The **API URL** can be found in your Wiz portal settings or documentation (varies by region: us17, us83, eu1, etc.)
   9. The **Auth Endpoint** is typically `https://auth.app.wiz.io/oauth/token` for all regions
   
   Documentation: https://docs.wiz.io/wiz-docs/docs/using-the-wiz-api (may be gated)
   
   **Does the credential need any specific scopes or permissions?**
   
   Yes, the service account must have the following permissions:
   
   * `read:users` - Required to sync user accounts via the `userAccounts` endpoint
   * `read:projects` - Required to sync projects and project memberships
   * `read:roles` - Required to sync user roles via the `userRolesV2` endpoint
   * `read:security_issues` or `read:issues` - Required to sync security insights/findings
   
   Note: The exact permission names may vary. In Wiz, these are typically granted by selecting "Read" access for Users, Projects, Roles, and Issues when creating the service account.
   
   **Is the list of scopes or permissions different to sync (read) versus provision (read-write)?**
   
   Yes. Sync needs only the read permissions above. Provisioning also needs `write:users` (role and project-member assignment) and `write:projects` (project owners and security champions).
   
   **What level of access or permissions does the user need in order to create the credentials?**
   
   The user must have permissions to create service accounts in Wiz. This typically requires:
   
   * **Admin** or **Project Admin** role in Wiz
   * Access to Settings > Service Accounts in the Wiz portal
   
   Note: Standard users typically cannot create service accounts. Contact your Wiz administrator if you don't have access to create service accounts.

## Known Limitations

The connector has the following known limitations due to Wiz API restrictions when using service account authentication:

1. **Single role per user**: Wiz users hold exactly one role. Granting a role replaces the user's current role, and a held role cannot be revoked, only replaced by granting another.

2. **Project assignment needs a project-scoped role**: Project `member` grants change the user's assigned projects, which only apply to users with a project-scoped role. Users with a global role already have access to every project, so the grant is refused for them.

3. **SSO-managed users**: For users whose role or projects come from SSO group mapping, provisioning writes a manual override in Wiz. The override replaces the SSO-derived value and persists until changed in Wiz, so later SSO group changes no longer apply to that field.

4. **User ID Inconsistency**: Wiz returns different user IDs for the same user across different endpoints:
   - The connector uses email addresses as the canonical user identifier to work around this
   - Users without email addresses are skipped during sync

5. **IAM-Focused Security Insights**: Security insights are filtered to only include issues affecting user principals (USER_ACCOUNT and SERVICE_ACCOUNT entities):
   - Infrastructure issues (VPCs, buckets, regions, etc.) are excluded
   - This is intentional to focus on identity-related security risks relevant to IAM governance  

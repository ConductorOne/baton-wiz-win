![Baton Logo](./baton-logo.png)

# `baton-wiz-win` [![Go Reference](https://pkg.go.dev/badge/github.com/conductorone/baton-wiz-win.svg)](https://pkg.go.dev/github.com/conductorone/baton-wiz-win) ![main ci](https://github.com/conductorone/baton-wiz-win/actions/workflows/main.yaml/badge.svg)

`baton-wiz-win` is a connector for Wiz.io cloud security platform built using the [Baton SDK](https://github.com/conductorone/baton-sdk). It syncs IAM resources (users, roles, projects) and security insights from Wiz to enable comprehensive identity and security posture management.

Check out [Baton](https://github.com/conductorone/baton) to learn more the project in general.

# Prerequisites

- **Wiz Account**: You need an active Wiz account with API access
- **OAuth2 Credentials**: Create an OAuth2 client in Wiz with the following permissions:
  - `read:users` - To sync user information
  - `read:projects` - To sync project/workspace information
  - `read:security_issues` - To sync security insights and findings
- **API Endpoints**: You'll need both the GraphQL API URL and the OAuth2 token endpoint for your Wiz region

# Getting Started

## brew

```bash
brew install conductorone/baton/baton conductorone/baton/baton-wiz-win

baton-wiz-win \
  --wiz-api-url "https://api.wiz.io/graphql" \
  --wiz-client-id "your-client-id" \
  --wiz-client-secret "your-client-secret" \
  --wiz-auth-endpoint "https://auth.wiz.io/oauth/token"

baton resources
```

## docker

```bash
docker run --rm -v $(pwd):/out \
  -e BATON_WIZ_API_URL="https://api.wiz.io/graphql" \
  -e BATON_WIZ_CLIENT_ID="your-client-id" \
  -e BATON_WIZ_CLIENT_SECRET="your-client-secret" \
  -e BATON_WIZ_AUTH_ENDPOINT="https://auth.wiz.io/oauth/token" \
  ghcr.io/conductorone/baton-wiz-win:latest -f "/out/sync.c1z"

docker run --rm -v $(pwd):/out \
  ghcr.io/conductorone/baton:latest -f "/out/sync.c1z" resources
```

## source

```bash
go install github.com/conductorone/baton/cmd/baton@main
go install github.com/conductorone/baton-wiz-win/cmd/baton-wiz-win@main

baton-wiz-win \
  --wiz-api-url "https://api.wiz.io/graphql" \
  --wiz-client-id "your-client-id" \
  --wiz-client-secret "your-client-secret" \
  --wiz-auth-endpoint "https://auth.wiz.io/oauth/token"

baton resources
```

# Data Model

`baton-wiz-win` synchronizes information about the following Wiz resources:

## IAM Resources
- **Users**: Wiz user accounts with email, name, status, and role assignments
- **Roles**: Wiz permission levels (Admin, Editor, Viewer, etc.) with member entitlements
- **Projects**: Wiz projects/workspaces with membership entitlements

## Security Resources
- **Security Insights**: Wiz security issues and findings mapped to external cloud resources (AWS, Azure, GCP)
  - Uses the `SecurityInsightTrait` to link Wiz issues to resources from other connectors
  - Includes severity, status, issue type, and affected resource information
  - Automatically detects cloud provider (AWS/Azure/GCP) from resource external IDs
  - Enables correlation of security findings with IAM access patterns in ConductorOne

## How Security Insights Work

Security Insights in this connector leverage the Baton SDK's `SecurityInsightTrait` to map Wiz findings to external cloud resources:

1. **Issue Discovery**: The connector fetches security issues from Wiz (vulnerabilities, misconfigurations, compliance violations)
2. **External Resource Mapping**: Each issue references a cloud resource via its external ID (e.g., AWS ARN, Azure Resource ID)
3. **Uplift Integration**: ConductorOne's Uplift system matches these external IDs to resources synced from other connectors (baton-aws, baton-azure, etc.)
4. **Unified View**: Security findings are displayed alongside IAM entitlements, enabling security teams to understand both "who has access" and "what risks exist"

`baton-wiz-win` does not currently support account provisioning or entitlement provisioning.

# Contributing, Support and Issues

We started Baton because we were tired of taking screenshots and manually
building spreadsheets. We welcome contributions, and ideas, no matter how
small&mdash;our goal is to make identity and permissions sprawl less painful for
everyone. If you have questions, problems, or ideas: Please open a GitHub Issue!

See [CONTRIBUTING.md](https://github.com/ConductorOne/baton/blob/main/CONTRIBUTING.md) for more details.

# `baton-wiz-win` Command Line Usage

```
baton-wiz-win

Usage:
  baton-wiz-win [flags]
  baton-wiz-win [command]

Available Commands:
  capabilities       Get connector capabilities
  completion         Generate the autocompletion script for the specified shell
  help               Help about any command

Flags:
  -f, --file string                  The path to the c1z file to sync with ($BATON_FILE) (default "sync.c1z")
  -h, --help                         help for baton-wiz-win
      --log-format string            The output format for logs: json, console ($BATON_LOG_FORMAT) (default "json")
      --log-level string             The log level: debug, info, warn, error ($BATON_LOG_LEVEL) (default "info")
  -p, --provisioning                 If this connector supports provisioning, this must be set in order for provisioning actions to be enabled ($BATON_PROVISIONING)
      --ticketing                    This must be set to enable ticketing support ($BATON_TICKETING)
  -v, --version                      version for baton-wiz-win
      --wiz-api-url string           required: The Wiz GraphQL API endpoint (e.g., https://api.wiz.io/graphql) ($BATON_WIZ_API_URL)
      --wiz-auth-endpoint string     required: OAuth2 token endpoint (e.g., https://auth.wiz.io/oauth/token) ($BATON_WIZ_AUTH_ENDPOINT)
      --wiz-client-id string         required: OAuth2 client ID for Wiz API authentication ($BATON_WIZ_CLIENT_ID)
      --wiz-client-secret string     required: OAuth2 client secret for Wiz API authentication ($BATON_WIZ_CLIENT_SECRET)

Use "baton-wiz-win [command] --help" for more information about a command.
```

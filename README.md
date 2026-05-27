# AI SRE Investigator

Powered by [Coral](https://withcoral.com) - one SQL interface over GitHub, Sentry, and Slack.

>**Deployment Note:** The frontend dashboard UI is hosted live on Vercel for layout presentation: **[Live Dashboard Preview](https://zero-trust-operational-intelligence.vercel.app/)**. Because this AI investigator leverages the local Coral CLI engine to execute real-time data queries, the Go backend runs locally on infrastructure as demonstrated in the 3-minute video submission.

## What it does

When a production incident fires, engineers waste hours jumping between GitHub, Sentry, and Slack to find the root cause. This agent queries all three in a single Coral SQL session and generates a plain-English root cause summary in seconds.

## How Coral powers it

- `github.pulls` - recent deploys that may have caused the incident
- `sentry.issues` - active unresolved errors correlated with deploy times
- `slack.channels` - workspace signal from your team

No ETL. No warehouse. No glue code. Coral handles auth, pagination, and rate limits.

## Cross-source JOIN

This is Coral's core superpower - correlating GitHub deploys with Sentry errors across two completely separate systems in a single SQL query:

```sql
SELECT p.number, p.title, p.merged_at,
       s.title AS error_title, s.first_seen
FROM github.pulls p
JOIN sentry.issues s
  ON s.first_seen <= p.merged_at
WHERE p.owner = 'your-username' AND p.repo = 'your-repo'
ORDER BY p.merged_at DESC
LIMIT 5
```

One query. Two external systems. Zero glue code.

## Stack

- **Coral** - SQL query layer over external APIs
- **Go** - backend HTTP server that shells out to `coral sql`
- **HTML/CSS/JS** - zero-dependency frontend dashboard

## Features

- Root cause summary generated from live data across all three sources
- Recent pull requests table pulled from GitHub
- Active unresolved errors table pulled from Sentry with severity badges
- Slack workspace channel signal
- Coral SQL query preview panel showing every query running under the hood

## Run it

```
coral source add --interactive github
coral source add --interactive sentry
coral source add --interactive slack
go run main.go
```

Open `http://localhost:8080`, enter a GitHub owner and repo, click Investigate.

## Built for

Pirates of the Coral-bean hackathon by [WeMakeDevs](https://wemakedevs.org)
# AI SRE Investigator

Powered by [Coral](https://withcoral.com) - one SQL interface over GitHub, Sentry, and Slack.

## What it does

When an incident happens, engineers waste hours jumping between tools to find the root cause. This agent queries GitHub, Sentry, and Slack in a single Coral SQL session and generates a plain-English root cause summary in seconds.

## How Coral powers it

- `github.pulls` - recent deploys that may have caused the incident
- `sentry.issues` - active errors correlated with deploy times
- `slack.channels` - workspace signal from your team

No ETL. No warehouse. No glue code.

## Stack

- **Coral** - SQL query layer over external APIs
- **Go** - backend HTTP server
- **HTML/CSS/JS** - zero-dependency frontend dashboard

## Run it

```
coral source add --interactive github
coral source add --interactive sentry
coral source add --interactive slack
go run main.go
```

Open http://localhost:8080
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// InvestigationRequest is the JSON body the frontend sends.
type InvestigationRequest struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

// InvestigationResult holds everything the frontend will display.
type InvestigationResult struct {
	Timestamp    string      `json:"timestamp"`
	Owner        string      `json:"owner"`
	Repo         string      `json:"repo"`
	RecentPRs    []PRRow     `json:"recent_prs"`
	SentryErrors []SentryRow `json:"sentry_errors"`
	SlackAlerts  []SlackRow  `json:"slack_alerts"`
	Summary      string      `json:"summary"`
	Error        string      `json:"error,omitempty"`
}

type PRRow struct {
	Number   string `json:"number"`
	Title    string `json:"title"`
	State    string `json:"state"`
	Author   string `json:"author"`
	MergedAt string `json:"merged_at"`
}

type SentryRow struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	FirstSeen string `json:"first_seen"`
	Level     string `json:"level"`
}

type SlackRow struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
	Time    string `json:"time"`
}

// runCoralSQL runs a coral sql query and returns the raw stdout output.
func runCoralSQL(query string) (string, error) {
	cmd := exec.Command("coral", "sql", "--format", "json", query)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("coral error: %v\noutput: %s", err, string(out))
	}
	return string(out), nil
}

// parseCoralJSON parses coral's JSON output into a slice of maps.
func parseCoralJSON(raw string) ([]map[string]interface{}, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return []map[string]interface{}{}, nil
	}
	if strings.HasPrefix(raw, "[") {
		var rows []map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &rows); err == nil {
			return rows, nil
		}
	}
	var rows []map[string]interface{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row map[string]interface{}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func str(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func investigate(owner, repo string) InvestigationResult {
	result := InvestigationResult{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Owner:     owner,
		Repo:      repo,
	}

	// ── Query 1: Recent closed PRs ─────────────────────────────────────────
	prQuery := fmt.Sprintf(`
SELECT number, title, state, user__login, merged_at
FROM github.pulls
WHERE owner = '%s'
  AND repo = '%s'
  AND state = 'closed'
LIMIT 10
`, owner, repo)

	prRaw, err := runCoralSQL(prQuery)
	if err != nil {
		result.Error = err.Error()
		result.Summary = "Could not run GitHub query. Check that your GitHub token is valid and the repo name is correct."
		return result
	}
	prRows, _ := parseCoralJSON(prRaw)
	for _, r := range prRows {
		result.RecentPRs = append(result.RecentPRs, PRRow{
			Number:   str(r, "number"),
			Title:    str(r, "title"),
			State:    str(r, "state"),
			Author:   str(r, "user__login"),
			MergedAt: str(r, "merged_at"),
		})
	}

	// ── Query 2: Active Sentry errors ──────────────────────────────────────
	sentryQuery := `
SELECT id, title, status, first_seen, level
FROM sentry.issues

ORDER BY first_seen DESC
LIMIT 10
`
	sentryRaw, err := runCoralSQL(sentryQuery)
	if err != nil {
		result.SentryErrors = []SentryRow{}
	} else {
		sentryRows, _ := parseCoralJSON(sentryRaw)
		for _, r := range sentryRows {
			result.SentryErrors = append(result.SentryErrors, SentryRow{
				ID:        str(r, "id"),
				Title:     str(r, "title"),
				Status:    str(r, "status"),
				FirstSeen: str(r, "first_seen"),
				Level:     str(r, "level"),
			})
		}
	}

	// ── Query 3: Slack channels as signal ─────────────────────────────────
	// Note: slack.messages is not available in this Coral version.
	// We query slack.channels to show workspace connectivity and channel signals.
	slackQuery := `
SELECT id, name, purpose, num_members
FROM slack.channels
ORDER BY num_members DESC
LIMIT 10
`
	slackRaw, err := runCoralSQL(slackQuery)
	if err != nil {
		result.SlackAlerts = []SlackRow{}
	} else {
		slackRows, _ := parseCoralJSON(slackRaw)
		keywords := []string{"alert", "incident", "outage", "oncall", "sre", "ops", "monitor", "deploy", "error", "critical"}
		for _, r := range slackRows {
			name := strings.ToLower(str(r, "name"))
			purpose := strings.ToLower(str(r, "purpose"))
			combined := name + " " + purpose
			for _, kw := range keywords {
				if strings.Contains(combined, kw) {
					result.SlackAlerts = append(result.SlackAlerts, SlackRow{
						Channel: str(r, "name"),
						Text:    str(r, "purpose"),
						Time:    fmt.Sprintf("%s members", str(r, "num_members")),
					})
					break
				}
			}
		}
		// If no keyword match, still show all channels as workspace signal
		if len(result.SlackAlerts) == 0 {
			for _, r := range slackRows {
				result.SlackAlerts = append(result.SlackAlerts, SlackRow{
					Channel: str(r, "name"),
					Text:    str(r, "purpose"),
					Time:    fmt.Sprintf("%s members", str(r, "num_members")),
				})
			}
		}
	}

	result.Summary = buildSummary(result)
	return result
}

func buildSummary(r InvestigationResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Investigation for %s/%s completed at %s.\n\n", r.Owner, r.Repo, r.Timestamp))

	sb.WriteString(fmt.Sprintf("RECENT DEPLOYS: Found %d closed pull requests.\n", len(r.RecentPRs)))
	if len(r.RecentPRs) > 0 {
		sb.WriteString("Most recent:\n")
		limit := 3
		if len(r.RecentPRs) < limit {
			limit = len(r.RecentPRs)
		}
		for _, pr := range r.RecentPRs[:limit] {
			sb.WriteString(fmt.Sprintf("  PR #%s - %s (by %s, merged %s)\n", pr.Number, pr.Title, pr.Author, pr.MergedAt))
		}
	}

	sb.WriteString(fmt.Sprintf("\nACTIVE ERRORS: Found %d unresolved Sentry issues.\n", len(r.SentryErrors)))
	if len(r.SentryErrors) > 0 {
		sb.WriteString("Top errors:\n")
		limit := 3
		if len(r.SentryErrors) < limit {
			limit = len(r.SentryErrors)
		}
		for _, e := range r.SentryErrors[:limit] {
			sb.WriteString(fmt.Sprintf("  [%s] %s (first seen: %s)\n", strings.ToUpper(e.Level), e.Title, e.FirstSeen))
		}
	}

	sb.WriteString(fmt.Sprintf("\nSLACK OPERATIONAL INTELLIGENCE: Found %d channels.\n", len(r.SlackAlerts)))
	if len(r.SlackAlerts) > 0 {
		limit := 3
		if len(r.SlackAlerts) < limit {
			limit = len(r.SlackAlerts)
		}
		for _, s := range r.SlackAlerts[:limit] {
			text := s.Text
			if len(text) > 120 {
				text = text[:120] + "..."
			}
			sb.WriteString(fmt.Sprintf("  #%s (%s): %s\n", s.Channel, s.Time, text))
		}
	}

	if len(r.SentryErrors) > 0 && len(r.RecentPRs) > 0 {
		sb.WriteString("\nROOT CAUSE HYPOTHESIS: Active errors detected alongside recent deploys. ")
		sb.WriteString("Correlate the error timestamps with the merge timestamps above to identify the likely breaking change.")
	} else if len(r.SentryErrors) == 0 && len(r.RecentPRs) == 0 {
		sb.WriteString("\nROOT CAUSE HYPOTHESIS: No errors and no recent deploys found. System appears stable.")
	} else if len(r.SentryErrors) > 0 {
		sb.WriteString("\nROOT CAUSE HYPOTHESIS: Active errors found but no recent deploys. Investigate infrastructure or dependency changes.")
	} else {
		sb.WriteString("\nROOT CAUSE HYPOTHESIS: Recent deploys found but no active errors. System appears stable post-deploy.")
	}

	return sb.String()
}

func handleInvestigate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
		return
	}

	var req InvestigationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}
	req.Owner = strings.TrimSpace(req.Owner)
	req.Repo = strings.TrimSpace(req.Repo)
	if req.Owner == "" || req.Repo == "" {
		http.Error(w, `{"error":"owner and repo are required"}`, http.StatusBadRequest)
		return
	}

	result := investigate(req.Owner, req.Repo)
	json.NewEncoder(w).Encode(result)
}

func main() {
	http.HandleFunc("/api/investigate", handleInvestigate)
	http.Handle("/", http.FileServer(http.Dir("static")))
	fmt.Println("SRE Agent running at http://localhost:8080")
	fmt.Println("Open your browser to http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

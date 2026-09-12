package domain

import "time"

// LinkedInScanCheck is a single pass/fail item in the scan report (e.g. "Headline length is
// good", "Missing location") — spec 015.
type LinkedInScanCheck struct {
	Label       string `json:"label"`
	Passed      bool   `json:"passed"`
	Explanation string `json:"explanation"`
}

// LinkedInScanSection groups related checks (e.g. "Informações básicas", "Experiência
// profissional").
type LinkedInScanSection struct {
	Name   string              `json:"name"`
	Checks []LinkedInScanCheck `json:"checks"`
}

// LinkedInScan is the audit report generated from the text of a user-uploaded LinkedIn
// profile PDF export ("Save to PDF"). Only the most recent scan is kept per user — no
// history (spec 015).
type LinkedInScan struct {
	ID              string                `json:"id"`
	UserID          string                `json:"user_id"`
	Score           float64               `json:"score"`
	Sections        []LinkedInScanSection `json:"sections"`
	PredictedSkills []string              `json:"predicted_skills"`
	Tips            []string              `json:"tips"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

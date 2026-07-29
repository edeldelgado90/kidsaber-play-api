package domain

import "time"

// ReportStatus is the review state of a question report.
type ReportStatus string

const (
	// ReportStatusOpen means the report still needs a human to look at it.
	ReportStatusOpen ReportStatus = "open"
	// ReportStatusResolved means the question was fixed or replaced.
	ReportStatusResolved ReportStatus = "resolved"
	// ReportStatusDismissed means the report was checked and the question is fine.
	ReportStatusDismissed ReportStatus = "dismissed"
)

// QuestionSummary is the subset of a stored question needed to record and
// describe a report. It is always read from the database, never taken from the
// request body: the client sends only a question id, so a caller cannot inject
// its own subject, grade or statement into the reports table or into Discord.
type QuestionSummary struct {
	ID        string
	Subject   Subject
	Grade     int
	Type      GameType
	Statement string
}

// QuestionReport is a player-submitted report that a question looks wrong.
// There is exactly one per question; repeat reports raise Count.
type QuestionReport struct {
	Question        QuestionSummary
	Count           int
	Status          ReportStatus
	FirstReportedAt time.Time
	LastReportedAt  time.Time
}

// ReportOutcome describes what recording a report actually did.
//
// Created distinguishes the first report for a question from a repeat, which is
// what lets the caller notify once per question instead of once per tap.
type ReportOutcome struct {
	Created bool
	Count   int
}

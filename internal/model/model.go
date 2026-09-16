// Package model holds the data types shared by every layer. JSON tags are the
// wire and on-disk format; changing them breaks saved jobs and exports.
package model

// Record is one business row from an uploaded file plus review state.
type Record struct {
	ID           string         `json:"id"`
	Number       string         `json:"number"`
	Enterprise   string         `json:"enterprise"`
	Kind         string         `json:"kind"`
	Name         string         `json:"name"`
	Address      string         `json:"address"`
	Municipality string         `json:"municipality"`
	Status       string         `json:"status"`
	Phone        string         `json:"phone"`
	Email        string         `json:"email"`
	Website      string         `json:"website"`
	Notes        string         `json:"notes"`
	Reviewed     bool           `json:"reviewed"`
	Issues       []string       `json:"issues"`
	Geometry     any            `json:"geometry"`
	Source       map[string]any `json:"source"`
	Google       *Place         `json:"google,omitempty"`
	GoogleError  string         `json:"googleError,omitempty"`
	Verification *Verification  `json:"verification,omitempty"`
	Suggestions  []Suggestion   `json:"suggestions,omitempty"`
}

// Evidence is one piece of support for a suggestion.
type Evidence struct {
	Source string `json:"source"`
	URL    string `json:"url,omitempty"`
	Note   string `json:"note,omitempty"`
}

// Suggestion kinds.
const (
	KindConfirmed = "confirmed"
	KindDifferent = "different"
	KindNew       = "new"
)

// Suggestion is a proposed value for one field, never applied automatically.
type Suggestion struct {
	Field      string     `json:"field"`
	Current    string     `json:"current"`
	Value      string     `json:"value"`
	Kind       string     `json:"kind"`
	Confidence float64    `json:"confidence"`
	Evidence   []Evidence `json:"evidence"`
	Accepted   bool       `json:"accepted,omitempty"`
	Auto       bool       `json:"auto,omitempty"` // applied automatically: high confidence, two sources agree
}

// Verdicts.
const (
	VerdictRunning = "running"
	VerdictActive  = "likely_active"
	VerdictCeased  = "likely_ceased"
	VerdictUnclear = "unclear"
	VerdictSkipped = "skipped"
	VerdictNotRun  = "not_run"
)

// Step statuses.
const (
	StepRunning = "running"
	StepDone    = "done"
	StepSkipped = "skipped"
	StepFailed  = "failed"
)

// Step is one stage of a record's verification, kept so reviewers can see
// what ran, what it found and what it cost.
type Step struct {
	Demo      *CompanywebDemo `json:"demo,omitempty"` // isolated fixture, never verification evidence
	Stage     string          `json:"stage"`          // checks | google | website | databe | goldenpages | trendstop | vkbo | judge | extract | reconcile
	Status    string          `json:"status"`
	Note      string          `json:"note"`
	Cost      float64         `json:"cost"`
	Findings  int             `json:"findings"`
	StartedAt string          `json:"startedAt"`
	EndedAt   string          `json:"endedAt,omitempty"`
}

// CompanywebDemo is a prepared snapshot for a hackathon, not an API response.
type CompanywebDemo struct {
	URL        string `json:"url"`
	CapturedAt string `json:"capturedAt"`
	Enterprise string `json:"enterprise"`
	Name       string `json:"name"`
	Address    string `json:"address"`
	Status     string `json:"status"`
}

// Verification is the real-world existence check for one record.
type Verification struct {
	Verdict   string   `json:"verdict"`
	Reason    string   `json:"reason"`
	Sources   []string `json:"sources"`
	Skipped   string   `json:"skipped,omitempty"`
	Cost      float64  `json:"cost"`
	CheckedAt string   `json:"checkedAt"`
	Trace     []Step   `json:"trace"`
}

// Enrichment tracks a job's verification run and budget.
type Enrichment struct {
	Sources     []string `json:"sources"`
	BudgetEUR   float64  `json:"budgetEur"`
	SpentEUR    float64  `json:"spentEur"`
	Verified    int      `json:"verified"`
	Skipped     int      `json:"skipped"`
	Concurrency int      `json:"concurrency"`
}

// Place is an unconfirmed Google Places candidate for a record.
type Place struct {
	ID          string `json:"id"`
	DisplayName struct {
		Text string `json:"text"`
	} `json:"displayName"`
	FormattedAddress string `json:"formattedAddress"`
	BusinessStatus   string `json:"businessStatus"`
	Phone            string `json:"internationalPhoneNumber"`
	Website          string `json:"websiteUri"`
	MapsURL          string `json:"googleMapsUri"`
	// Industry as Google classifies the listing: the primary type's display
	// name (e.g. "Bakery") plus the raw type identifiers.
	PrimaryType        string `json:"primaryType,omitempty"`
	PrimaryTypeDisplay struct {
		Text string `json:"text"`
	} `json:"primaryTypeDisplayName"`
	Types []string `json:"types,omitempty"`
}

// Industry returns the human-readable Google category, or "".
func (p *Place) Industry() string {
	if p == nil {
		return ""
	}
	if p.PrimaryTypeDisplay.Text != "" {
		return p.PrimaryTypeDisplay.Text
	}
	return p.PrimaryType
}

// Job states.
const (
	StateProcessing = "processing"
	StateDone       = "done"
	StateFailed     = "failed"
)

// Notification states.
const (
	NotifyNotRequested = "not requested"
	NotifyPending      = "pending"
	NotifySending      = "sending"
	NotifySent         = "sent"
	NotifyFailed       = "failed"
	NotifyUnknown      = "delivery unknown"
)

// Job is one uploaded file and its processing outcome.
type Job struct {
	ID           string      `json:"id"`
	Filename     string      `json:"filename"`
	Created      string      `json:"created"`
	State        string      `json:"state"`
	Phase        string      `json:"phase"`
	Progress     int         `json:"progress"`
	Total        int         `json:"total"`
	Records      []Record    `json:"records"`
	Email        string      `json:"email"`
	Notification string      `json:"notification"`
	Enrich       bool        `json:"enrich"`
	Enrichment   *Enrichment `json:"enrichment,omitempty"`
	Error        string      `json:"error,omitempty"`
}

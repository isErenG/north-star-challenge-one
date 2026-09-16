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
	// DuplicateGroup, MergedInto and MergedFrom are set by internal/dedupe.
	// DuplicateGroup is the canonical record's ID, shared by every member of
	// a near-duplicate cluster. MergedInto is set on a merged-away record to
	// the same ID; the canonical keeps MergedInto empty and lists what it
	// absorbed in MergedFrom (their number, or name if the number was blank).
	DuplicateGroup string   `json:"duplicateGroup,omitempty"`
	MergedInto     string   `json:"mergedInto,omitempty"`
	MergedFrom     []string `json:"mergedFrom,omitempty"`
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
	ID           string   `json:"id"`
	Filename     string   `json:"filename"`
	Created      string   `json:"created"`
	State        string   `json:"state"`
	Phase        string   `json:"phase"`
	Progress     int      `json:"progress"`
	Total        int      `json:"total"`
	Records      []Record `json:"records"`
	Email        string   `json:"email"`
	Notification string   `json:"notification"`
	Enrich       bool     `json:"enrich"`
	Error        string   `json:"error,omitempty"`
}

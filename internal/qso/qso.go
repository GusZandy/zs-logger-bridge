// Package qso defines the bridge's normalized contact shape. Both the N1MM
// (XML) and JTDX/WSJT-X (UDP binary) listeners parse their own wire formats
// and produce one of these, so everything downstream -- dedupe, upload, the
// UI activity feed -- only ever deals with one shape.
package qso

import (
	"fmt"
	"strings"
	"time"
)

// Source identifies which logging program a QSO came from.
type Source string

const (
	SourceN1MM Source = "n1mm"
	SourceJTDX Source = "jtdx"
)

// QSO is a single normalized contact, ready to be deduped and uploaded.
type QSO struct {
	Callsign     string
	FrequencyMHz float64
	Mode         string
	RST          string // report received
	SentRST      string // report sent
	Gridlocator  string
	Note         string
	LoggedAt     time.Time // always UTC
	Source       Source
}

// Key is a stable identity for dedupe purposes: callsign + mode + the
// frequency rounded to a band-ish bucket, independent of exact timestamp.
// Time is handled separately (a window, not exact match) by the dedupe
// cache and again server-side, since two radios logging the "same" QSO a
// couple of seconds apart is common and expected.
func (q QSO) Key() string {
	return fmt.Sprintf("%s|%s|%.3f", strings.ToUpper(strings.TrimSpace(q.Callsign)), strings.ToUpper(q.Mode), q.FrequencyMHz)
}

// IngestPayload is the JSON body posted to
// POST /api/logsheets/{id}/qso on the logger backend. Field names match
// BridgeIngestController::rules() on the server exactly.
type IngestPayload struct {
	Callsign    string `json:"callsign"`
	Frequency   string `json:"frequency"`
	Mode        string `json:"mode"`
	RST         string `json:"rst"`
	SentRST     string `json:"sent_rst"`
	Gridlocator string `json:"gridlocator,omitempty"`
	Note        string `json:"note,omitempty"`
	LoggedAt    string `json:"logged_at"`
	Source      string `json:"source,omitempty"`
}

// ToPayload converts to the wire format the logger backend expects.
// Frequency is sent in MHz (e.g. "14.074000") to match Band::fromFrequency()
// on the server, which was built around N1MM/N1MM+-style MHz strings.
// LoggedAt is sent as "Y-m-d H:i:s" UTC, which Laravel's `date` validation
// rule accepts directly.
// NOTE: the logger's `rst`/`sent_rst` columns are validated server-side as
// max:3 chars, which fits SSB/CW reports ("59", "599") but not every WSJT-X
// digital-mode exchange (e.g. "R-15" is 4 chars). We truncate defensively so
// a QSO never gets rejected outright; if you want full-fidelity FT8/FT4
// exchange reports, bump `rst`/`sent_rst` to max:5 in
// BridgeIngestController::rules() (and LogController::rules(), to match) on
// the logger side.
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (q QSO) ToPayload() IngestPayload {
	note := q.Note
	if len(note) > 20 {
		note = note[:20]
	}

	return IngestPayload{
		Callsign:    strings.ToUpper(strings.TrimSpace(q.Callsign)),
		Frequency:   fmt.Sprintf("%.6f", q.FrequencyMHz),
		Mode:        q.Mode,
		RST:         truncate(q.RST, 3),
		SentRST:     truncate(q.SentRST, 3),
		Gridlocator: q.Gridlocator,
		Note:        note,
		LoggedAt:    q.LoggedAt.UTC().Format("2006-01-02 15:04:05"),
		Source:      string(q.Source),
	}
}

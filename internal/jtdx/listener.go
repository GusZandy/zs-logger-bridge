// Package jtdx listens for JTDX/WSJT-X's UDP protocol and turns
// "QSO Logged" messages into normalized qso.QSO values.
//
// JTDX is a WSJT-X fork and speaks the exact same UDP protocol, so this
// package leans entirely on github.com/k0swe/wsjtx-go, a maintained Go
// binding for that protocol (the same protocol Ham Radio Deluxe, GridTracker,
// etc. use), rather than hand-parsing WSJT-X's binary QDataStream framing.
//
// Enable it in JTDX/WSJT-X under Settings -> Reporting -> "Enable
// broadcasting" (or similar wording depending on version), pointed at this
// bridge's IP and the configured port (default 2237).
package jtdx

import (
	"fmt"
	"net"
	"strings"

	wsjtx "github.com/k0swe/wsjtx-go/v4"

	"github.com/yc2utc/zs-logger-bridge/internal/qso"
)

// Listener binds a UDP port (as a WSJT-X "server", in that library's
// terminology -- WSJT-X is actually the client sending to us) and emits
// normalized QSOs.
type Listener struct {
	Port int

	// OnQSO is called for every "QSO Logged" message.
	OnQSO func(qso.QSO)
	// OnError is called for parse/transport errors. Non-fatal.
	OnError func(error)
	// OnRawEvent is called for every other recognized message type
	// (Heartbeat, Status, Decode, ...), useful for a "last activity" /
	// "connected" indicator in the UI even though v1 only acts on QSOs.
	OnRawEvent func(kind string)
}

// Start binds the configured port and blocks, processing messages until the
// connection is closed or done is closed. Run it in a goroutine.
func (l *Listener) Start(done <-chan struct{}) error {
	server, err := wsjtx.MakeServerGiven(net.IPv4zero, uint(l.Port))
	if err != nil {
		return fmt.Errorf("jtdx: bind :%d: %w", l.Port, err)
	}

	msgs := make(chan interface{})
	errs := make(chan error)

	go server.ListenToWsjtx(msgs, errs)

	go func() {
		<-done
		_ = server.Shutdown()
	}()

	for {
		select {
		case <-done:
			return nil
		case err, ok := <-errs:
			if !ok {
				return nil
			}
			l.reportError(fmt.Errorf("jtdx: %w", err))
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			l.handleMessage(msg)
		}
	}
}

func (l *Listener) handleMessage(msg interface{}) {
	switch m := msg.(type) {
	case wsjtx.QsoLoggedMessage:
		q := normalize(m)
		if l.OnQSO != nil {
			l.OnQSO(q)
		}
	case wsjtx.HeartbeatMessage:
		l.reportRaw("heartbeat")
	case wsjtx.StatusMessage:
		l.reportRaw("status")
	case wsjtx.DecodeMessage:
		l.reportRaw("decode")
	case wsjtx.LoggedAdifMessage:
		// WSJT-X/JTDX sends both QsoLoggedMessage and LoggedAdifMessage for
		// the same event when a QSO is logged. We act on QsoLoggedMessage
		// only (it's already structured) and treat this one as informational
		// to avoid double-forwarding the same contact.
		l.reportRaw("logged_adif")
	default:
		l.reportRaw("other")
	}
}

func (l *Listener) reportRaw(kind string) {
	if l.OnRawEvent != nil {
		l.OnRawEvent(kind)
	}
}

func (l *Listener) reportError(err error) {
	if l.OnError != nil {
		l.OnError(err)
	}
}

// normalize converts a wsjtx.QsoLoggedMessage into a qso.QSO.
func normalize(m wsjtx.QsoLoggedMessage) qso.QSO {
	freqMHz := float64(m.TxFrequency) / 1_000_000

	note := strings.TrimSpace(m.Comments)
	if note == "" {
		note = strings.TrimSpace(m.Name)
	}

	loggedAt := m.DateTimeOff
	if loggedAt.IsZero() {
		loggedAt = m.DateTimeOn
	}

	return qso.QSO{
		Callsign:     strings.ToUpper(strings.TrimSpace(m.DxCall)),
		FrequencyMHz: freqMHz,
		Mode:         strings.ToUpper(strings.TrimSpace(m.Mode)),
		RST:          strings.TrimSpace(m.ReportReceived),
		SentRST:      strings.TrimSpace(m.ReportSent),
		Gridlocator:  strings.ToUpper(strings.TrimSpace(m.DxGrid)),
		Note:         note,
		LoggedAt:     loggedAt.UTC(),
		Source:       qso.SourceJTDX,
	}
}

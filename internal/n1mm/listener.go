// Package n1mm listens for N1MM Logger+'s UDP broadcast and turns
// <contactinfo> datagrams into normalized qso.QSO values.
//
// N1MM is configured under Config -> Configure Ports... -> Broadcast Data,
// where the operator sets one or more destination IP:port pairs. This
// listener binds one of those ports and expects small, complete XML
// documents, one per datagram -- N1MM does not fragment a single
// <contactinfo> across packets.
//
// Besides <contactinfo> (a QSO was logged), N1MM also broadcasts
// <contactreplace> (a QSO was edited), <contactdelete> (a QSO was removed),
// <RadioInfo>, and <Spot>. v1 of the bridge only forwards new contacts and
// ignores the rest -- see the package comment on Listener.Start.
package n1mm

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/yc2utc/zs-logger-bridge/internal/qso"
)

// contactInfo mirrors the fields we care about in N1MM's <contactinfo>
// broadcast. Only a subset of N1MM's actual fields are declared; unknown
// elements are ignored by encoding/xml automatically.
type contactInfo struct {
	XMLName    xml.Name `xml:"contactinfo"`
	Call       string   `xml:"call"`
	Mode       string   `xml:"mode"`
	Band       string   `xml:"band"`
	RxFreq     string   `xml:"rxfreq"` // tens of Hz, e.g. 710000 = 7,100.00 kHz
	TxFreq     string   `xml:"txfreq"`
	Snt        string   `xml:"snt"` // RST sent
	Rcv        string   `xml:"rcv"` // RST received
	GridSquare string   `xml:"gridsquare"`
	MyCall     string   `xml:"mycall"`
	Operator   string   `xml:"operator"`
	Timestamp  string   `xml:"timestamp"` // "2006-01-02 15:04:05", UTC
}

// Listener binds a UDP port and emits normalized QSOs.
type Listener struct {
	Port int

	// OnQSO is called for every successfully parsed <contactinfo>.
	OnQSO func(qso.QSO)
	// OnError is called for anything that couldn't be parsed or handled.
	// Non-fatal; the listener keeps running.
	OnError func(error)
	// OnRawEvent is called for every recognized-but-ignored element
	// (contactreplace, contactdelete, RadioInfo, Spot), useful for a
	// "last activity" UI even though v1 doesn't act on them.
	OnRawEvent func(kind string)
}

// Start binds the configured port and blocks, processing datagrams until
// the connection is closed or done is closed. Run it in a goroutine.
func (l *Listener) Start(done <-chan struct{}) error {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: l.Port})
	if err != nil {
		return fmt.Errorf("n1mm: listen on :%d: %w", l.Port, err)
	}
	defer conn.Close()

	go func() {
		<-done
		conn.Close()
	}()

	buf := make([]byte, 65535)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			// Expected when done closes the connection.
			select {
			case <-done:
				return nil
			default:
			}
			l.reportError(fmt.Errorf("n1mm: read: %w", err))
			continue
		}

		l.handleDatagram(buf[:n])
	}
}

func (l *Listener) handleDatagram(data []byte) {
	dec := xml.NewDecoder(bytes.NewReader(data))

	tok, err := dec.Token()
	for err == nil {
		if start, ok := tok.(xml.StartElement); ok {
			l.dispatch(start.Name.Local, dec, start)
			return
		}
		tok, err = dec.Token()
	}
}

func (l *Listener) dispatch(rootName string, dec *xml.Decoder, start xml.StartElement) {
	switch rootName {
	case "contactinfo":
		var ci contactInfo
		if err := dec.DecodeElement(&ci, &start); err != nil {
			l.reportError(fmt.Errorf("n1mm: parse contactinfo: %w", err))
			return
		}
		q, err := normalize(ci)
		if err != nil {
			l.reportError(fmt.Errorf("n1mm: normalize contactinfo: %w", err))
			return
		}
		if l.OnQSO != nil {
			l.OnQSO(q)
		}
	case "contactreplace", "contactdelete", "RadioInfo", "Spot":
		// v1: not forwarded. See package comment.
		if l.OnRawEvent != nil {
			l.OnRawEvent(rootName)
		}
	default:
		// Unknown/future N1MM broadcast type; ignore quietly.
	}
}

func (l *Listener) reportError(err error) {
	if l.OnError != nil {
		l.OnError(err)
	}
}

// normalize converts N1MM's contactinfo into a qso.QSO.
func normalize(ci contactInfo) (qso.QSO, error) {
	call := strings.ToUpper(strings.TrimSpace(ci.Call))
	if call == "" {
		return qso.QSO{}, fmt.Errorf("empty call")
	}

	// rxfreq is documented by N1MM as "tens of Hz", e.g. 710000 means
	// 7,100.00 kHz = 7.1 MHz. MHz = rxfreq * 10 / 1_000_000.
	freqMHz := 0.0
	if ci.RxFreq != "" {
		tensOfHz, err := strconv.ParseFloat(strings.TrimSpace(ci.RxFreq), 64)
		if err != nil {
			return qso.QSO{}, fmt.Errorf("bad rxfreq %q: %w", ci.RxFreq, err)
		}
		freqMHz = tensOfHz * 10 / 1_000_000
	}

	loggedAt := time.Now().UTC()
	if ci.Timestamp != "" {
		t, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(ci.Timestamp), time.UTC)
		if err == nil {
			loggedAt = t
		}
	}

	return qso.QSO{
		Callsign:     call,
		FrequencyMHz: freqMHz,
		Mode:         strings.ToUpper(strings.TrimSpace(ci.Mode)),
		RST:          strings.TrimSpace(ci.Rcv),
		SentRST:      strings.TrimSpace(ci.Snt),
		Gridlocator:  strings.ToUpper(strings.TrimSpace(ci.GridSquare)),
		LoggedAt:     loggedAt,
		Source:       qso.SourceN1MM,
	}, nil
}

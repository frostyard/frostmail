package compose

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/mimex"
)

func TestBuildCalendar(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nMETHOD:REPLY\r\nBEGIN:VEVENT\r\nUID:x\r\nATTENDEE;CN=Zoë;PARTSTAT=ACCEPTED:mailto:a@example.com\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	m := Message{From: Address{Name: "Ann", Addr: "a@example.com"}, To: []Address{{Addr: "maria@example.com"}},
		Subject: "Accepted: Launch review", MessageID: "r1@example.com", Date: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC),
		HTML: "<p>Ann has accepted.</p>", Calendar: &Calendar{Method: "REPLY", Data: []byte(ics)}}
	var buf bytes.Buffer
	if err := Build(&buf, m); err != nil {
		t.Fatal(err)
	}
	var types []string
	var got []byte
	if err := mimex.WalkParts(buf.Bytes(), func(p mimex.PartInfo, body io.Reader) error {
		types = append(types, p.ContentType)
		if p.ContentType == "text/calendar" {
			got, _ = io.ReadAll(body)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(types, ",") != "text/plain,text/html,text/calendar" || string(got) != ics {
		t.Errorf("parts %q, calendar %q", types, got)
	}
	if !strings.Contains(buf.String(), `method=REPLY`) {
		t.Errorf("no method parameter:\n%s", buf.String())
	}
	for _, line := range strings.Split(buf.String(), "\r\n") {
		for _, r := range line {
			if r > 127 {
				t.Fatalf("an 8-bit line: %q", line)
			}
		}
	}
	m.Calendar.Method = "reply; x=y"
	if err := Build(io.Discard, m); !errors.Is(err, ErrInvalid) {
		t.Errorf("a bad method: %v", err)
	}
}

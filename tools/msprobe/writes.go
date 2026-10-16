package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// writes makes probe events and a probe contact in the account's default
// calendar and contacts, changes them the ways Frostmail will, reads back
// what Graph and its delta queries report, and deletes all of them. Every
// subject starts with "FM probe" and the contact is "FM Probe"; nothing
// else is touched. The events fall on October 26–30, 2026.
func (p *probe) writes(ctx context.Context) (err error) {
	tok, _, err := p.access(ctx, graphScopes)
	if err != nil {
		return err
	}
	if p.out != "" {
		if err := os.MkdirAll(p.out, 0o700); err != nil {
			return err
		}
	}
	g := writer{graphClient: graphClient{p: p, token: tok}}
	defer func() {
		if cerr := g.cleanup(context.WithoutCancel(ctx)); cerr != nil && err == nil {
			err = cerr
		}
	}()
	utc := map[string]string{"Prefer": `outlook.timezone="UTC"`}
	immutable := map[string]string{"Prefer": `outlook.timezone="UTC", IdType="ImmutableId"`}
	window := url.Values{"startDateTime": {"2026-10-25T00:00:00Z"}, "endDateTime": {"2026-11-02T00:00:00Z"}}

	// A delta round before the writes, so the next round shows only them.
	calDelta := g.deltaLink(ctx, "w-calview-delta-before", "/me/calendarView/delta?"+window.Encode(), immutable)
	conDelta := g.deltaLink(ctx, "w-contacts-delta-before", "/me/contacts/delta", map[string]string{"Prefer": `IdType="ImmutableId"`})

	// A single event, written with an IANA zone.
	single := g.create(ctx, "w-single", "/me/events", map[string]any{
		"subject":      "FM probe single",
		"start":        map[string]any{"dateTime": "2026-10-27T10:00:00", "timeZone": "America/New_York"},
		"end":          map[string]any{"dateTime": "2026-10-27T11:00:00", "timeZone": "America/New_York"},
		"isReminderOn": true, "reminderMinutesBeforeStart": 10,
		"body": map[string]any{"contentType": "text", "content": "Frostmail probe; deleted at the end."},
	}, nil)
	g.show(single, "originalStartTimeZone", "originalEndTimeZone", "start", "changeKey", "@odata.etag", "iCalUId", "isReminderOn", "reminderMinutesBeforeStart")
	// Its ID as an immutable ID.
	g.get(ctx, "w-single-immutable", "/me/events/"+url.PathEscape(str(single, "id"))+"?$select=id,subject", immutable)

	// A stale change key: does Graph refuse the write?
	stale := str(single, "@odata.etag")
	g.patch(ctx, "w-single-move", "/me/events/"+url.PathEscape(str(single, "id")), map[string]any{
		"start": map[string]any{"dateTime": "2026-10-27T11:00:00", "timeZone": "America/New_York"},
		"end":   map[string]any{"dateTime": "2026-10-27T12:00:00", "timeZone": "America/New_York"},
	}, map[string]string{"If-Match": stale})
	g.patch(ctx, "w-single-stale", "/me/events/"+url.PathEscape(str(single, "id")), map[string]any{
		"subject": "FM probe single, stale",
	}, map[string]string{"If-Match": stale})

	// An all-day event.
	g.create(ctx, "w-allday", "/me/events", map[string]any{
		"subject": "FM probe all day", "isAllDay": true,
		"start": map[string]any{"dateTime": "2026-10-28T00:00:00", "timeZone": "America/New_York"},
		"end":   map[string]any{"dateTime": "2026-10-29T00:00:00", "timeZone": "America/New_York"},
	}, utc)

	// A daily series of five; the second occurrence moved, the third
	// deleted, then the series renamed.
	series := g.create(ctx, "w-series", "/me/events", map[string]any{
		"subject": "FM probe series",
		"start":   map[string]any{"dateTime": "2026-10-26T09:30:00", "timeZone": "Eastern Standard Time"},
		"end":     map[string]any{"dateTime": "2026-10-26T09:45:00", "timeZone": "Eastern Standard Time"},
		"recurrence": map[string]any{
			"pattern": map[string]any{"type": "daily", "interval": 1},
			"range":   map[string]any{"type": "numbered", "startDate": "2026-10-26", "numberOfOccurrences": 5},
		},
	}, utc)
	g.show(series, "type", "recurrence", "originalStartTimeZone", "iCalUId")
	master := str(series, "id")
	inst := values(g.get(ctx, "w-series-instances", "/me/events/"+url.PathEscape(master)+"/instances?"+window.Encode(), utc))
	if len(inst) != 5 {
		return fmt.Errorf("the series has %d instances, not 5", len(inst))
	}
	g.show(inst[1], "id", "type", "occurrenceId", "seriesMasterId", "start", "originalStart")
	g.patch(ctx, "w-series-move-second", "/me/events/"+url.PathEscape(str(inst[1], "id")), map[string]any{
		"start": map[string]any{"dateTime": "2026-10-27T10:00:00", "timeZone": "Eastern Standard Time"},
		"end":   map[string]any{"dateTime": "2026-10-27T10:15:00", "timeZone": "Eastern Standard Time"},
	}, utc)
	g.del(ctx, "w-series-delete-third", "/me/events/"+url.PathEscape(str(inst[2], "id")), false)
	g.patch(ctx, "w-series-rename", "/me/events/"+url.PathEscape(master), map[string]any{"subject": "FM probe series, renamed"}, utc)
	after := values(g.get(ctx, "w-series-instances-after", "/me/events/"+url.PathEscape(master)+"/instances?"+window.Encode(), utc))
	for i, e := range after {
		p.printf("  after: instance %d %s %v %v\n", i, e["type"], dig(e, "start", "dateTime"), e["subject"])
	}
	// What the master says about its exceptions and cancellations.
	g.get(ctx, "w-series-master-occurrences", "/me/events/"+url.PathEscape(master)+"?$select=subject,recurrence,cancelledOccurrences,exceptionOccurrences", utc)
	g.get(ctx, "w-series-master-occurrences-beta", "https://graph.microsoft.com/beta/me/events/"+url.PathEscape(master)+"?$select=subject,recurrence,cancelledOccurrences,exceptionOccurrences", utc)
	g.get(ctx, "w-series-exceptions", "/me/events?$filter="+url.QueryEscape("seriesMasterId eq '"+master+"'"), utc)

	// The calendar delta after the writes.
	if calDelta != "" {
		g.pages(ctx, "w-calview-delta-after", calDelta, immutable)
	}

	// A contact: made, changed, given a photo.
	contact := g.create(ctx, "w-contact", "/me/contacts", map[string]any{
		"givenName": "FM", "surname": "Probe", "companyName": "Frostyard",
		"emailAddresses": []any{map[string]any{"address": "fm.probe@example.com", "name": "FM Probe"}},
		"mobilePhone":    "+1 555 0100",
	}, map[string]string{"Prefer": `IdType="ImmutableId"`})
	cid := str(contact, "id")
	g.patch(ctx, "w-contact-title", "/me/contacts/"+url.PathEscape(cid), map[string]any{"jobTitle": "Tester"}, nil)
	g.putPhoto(ctx, "/me/contacts/"+url.PathEscape(cid)+"/photo/$value")
	g.get(ctx, "w-contact-photo-meta", "/me/contacts/"+url.PathEscape(cid)+"/photo", nil)
	if conDelta != "" {
		g.pages(ctx, "w-contacts-delta-after", conDelta, map[string]string{"Prefer": `IdType="ImmutableId"`})
	}
	return g.err
}

// writer remembers what it made, to delete it at the end.
type writer struct {
	graphClient
	made []string // event and contact paths, newest last
	err  error
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func (g *writer) show(m map[string]any, ks ...string) {
	for _, k := range ks {
		g.p.printf("  %s: %v\n", k, m[k])
	}
}

// send makes a request with a JSON body and returns the decoded reply.
func (g *writer) send(ctx context.Context, name, method, path string, body any, headers map[string]string) (int, map[string]any) {
	u := path
	if strings.HasPrefix(path, "/") {
		u = graphURL + path
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			g.fail(err)
			return 0, nil
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		g.fail(err)
		return 0, nil
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := g.p.http.Do(req)
	if err != nil {
		g.fail(err)
		return 0, nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if g.p.out != "" && len(b) > 0 {
		_ = os.WriteFile(filepath.Join(g.p.out, name+".json"), b, 0o600)
	}
	g.p.printf("== %s: %s → HTTP %d", name, method, resp.StatusCode)
	if resp.StatusCode >= 300 {
		g.p.printf(": %s", strings.TrimSpace(string(b[:min(len(b), 300)])))
	}
	g.p.printf("\n")
	var doc map[string]any
	_ = json.Unmarshal(b, &doc)
	return resp.StatusCode, doc
}

func (g *writer) fail(err error) {
	if g.err == nil {
		g.err = err
	}
	g.p.printf("  error: %v\n", err)
}

// create posts a new object and remembers it for the cleanup.
func (g *writer) create(ctx context.Context, name, path string, body any, headers map[string]string) map[string]any {
	status, doc := g.send(ctx, name, http.MethodPost, path, body, headers)
	if status != http.StatusCreated {
		g.fail(fmt.Errorf("%s: HTTP %d", name, status))
		return map[string]any{}
	}
	g.made = append(g.made, path+"/"+url.PathEscape(str(doc, "id")))
	return doc
}

func (g *writer) patch(ctx context.Context, name, path string, body any, headers map[string]string) map[string]any {
	_, doc := g.send(ctx, name, http.MethodPatch, path, body, headers)
	return doc
}

func (g *writer) del(ctx context.Context, name, path string, quiet bool) int {
	status, _ := g.send(ctx, name, http.MethodDelete, path, nil, nil)
	if status != http.StatusNoContent && status != http.StatusNotFound && !quiet {
		g.fail(fmt.Errorf("%s: HTTP %d", name, status))
	}
	return status
}

// onePixel is a 1×1 PNG.
const onePixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="

func (g *writer) putPhoto(ctx context.Context, path string) {
	png, _ := base64.StdEncoding.DecodeString(onePixel)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, graphURL+path, bytes.NewReader(png))
	if err != nil {
		g.fail(err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", "image/png")
	resp, err := g.p.http.Do(req)
	if err != nil {
		g.fail(err)
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	g.p.printf("== w-contact-photo: PUT → HTTP %d %s\n", resp.StatusCode, strings.TrimSpace(string(b[:min(len(b), 300)])))
}

// deltaLink runs a delta round to its end and returns its delta link.
func (g *writer) deltaLink(ctx context.Context, name, path string, headers map[string]string) string {
	for i := 0; path != "" && i < 50; i++ {
		doc := g.get(ctx, fmt.Sprintf("%s-%d", name, i), path, headers)
		if doc == nil {
			return ""
		}
		if next, ok := doc["@odata.nextLink"].(string); ok {
			path = next
			continue
		}
		link, _ := doc["@odata.deltaLink"].(string)
		return link
	}
	return ""
}

// cleanup deletes what the probe made, newest first, and checks it is gone.
func (g *writer) cleanup(ctx context.Context) error {
	var errs []error
	for i := len(g.made) - 1; i >= 0; i-- {
		path := g.made[i]
		if status := g.del(ctx, "w-cleanup", path, true); status != http.StatusNoContent && status != http.StatusNotFound {
			errs = append(errs, fmt.Errorf("delete %s: HTTP %d", path, status))
		}
	}
	// Anything with the probe's name left in the window.
	left := g.get(ctx, "w-left", "/me/calendarView?startDateTime=2026-10-25T00:00:00Z&endDateTime=2026-11-02T00:00:00Z&$top=100", nil)
	for _, e := range values(left) {
		if strings.HasPrefix(str(e, "subject"), "FM probe") {
			errs = append(errs, fmt.Errorf("left: %v at %v", e["subject"], dig(e, "start", "dateTime")))
		}
	}
	contacts := g.get(ctx, "w-left-contacts", "/me/contacts?$top=100", nil)
	for _, c := range values(contacts) {
		if str(c, "displayName") == "FM Probe" {
			errs = append(errs, fmt.Errorf("left: contact %v", c["displayName"]))
		}
	}
	if len(errs) == 0 {
		g.p.printf("cleanup: everything the probe made is gone\n")
	}
	return errors.Join(errs...)
}

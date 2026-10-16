package graph

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Graph's writes (ADR-0040; docs/design/pim.md, Microsoft Graph): events
// and contacts are created with POST and changed with PATCH, sending only
// the fields that changed; an If-Match on the change key Frostmail last
// read makes a change to an item someone else changed fail with
// ErrPrecondition. A DELETE moves the item to Deleted Items. A contact's
// photo can be replaced but not deleted (405).

func (c *Client) send(ctx context.Context, method, path string, fields any, ifMatch string) (jsontext.Value, error) {
	var body []byte
	if fields != nil {
		var err error
		if body, err = json.Marshal(fields); err != nil {
			return nil, fmt.Errorf("graph: %s %s: %w", method, path, err)
		}
	}
	var headers map[string]string
	if ifMatch != "" {
		headers = map[string]string{"If-Match": ifMatch}
	}
	data, _, err := c.Do(ctx, method, path, body, headers)
	return jsontext.Value(data), err
}

func eventPath(id string) string { return "me/events/" + url.PathEscape(id) }

func contactPath(id string) string { return "me/contacts/" + url.PathEscape(id) }

// CreateEvent creates an event or a series in a calendar and returns it.
func (c *Client) CreateEvent(ctx context.Context, calendarID string, fields map[string]any) (Event, error) {
	raw, err := c.send(ctx, http.MethodPost, "me/calendars/"+url.PathEscape(calendarID)+"/events", fields, "")
	if err != nil {
		return Event{}, err
	}
	var e Event
	if err := json.Unmarshal(raw, &e); err != nil {
		return Event{}, fmt.Errorf("graph: created event: %w", err)
	}
	return e, nil
}

// UpdateEvent changes an event's fields, on its change key when ifMatch
// is set.
func (c *Client) UpdateEvent(ctx context.Context, id string, fields map[string]any, ifMatch string) error {
	_, err := c.send(ctx, http.MethodPatch, eventPath(id), fields, ifMatch)
	return err
}

// DeleteEvent deletes an event, a series or one occurrence, on its change
// key when ifMatch is set.
func (c *Client) DeleteEvent(ctx context.Context, id, ifMatch string) error {
	_, err := c.send(ctx, http.MethodDelete, eventPath(id), nil, ifMatch)
	return err
}

// Answers are the actions that answer an invitation, by PARTSTAT.
var answers = map[string]string{"accepted": "accept", "tentative": "tentativelyAccept", "declined": "decline"}

// Answer answers an invitation, or one occurrence of it, for the user,
// telling the organizer. answer is a PARTSTAT: accepted, tentative or
// declined.
func (c *Client) Answer(ctx context.Context, id, answer string) error {
	action, ok := answers[answer]
	if !ok {
		return fmt.Errorf("%w: the answer %q", ErrUnsupported, answer)
	}
	_, err := c.send(ctx, http.MethodPost, eventPath(id)+"/"+action, map[string]any{"sendResponse": true}, "")
	return err
}

// Instances lists a series' occurrences and exceptions that start in a
// window.
func (c *Client) Instances(ctx context.Context, masterID string, from, to time.Time) ([]Event, error) {
	q := url.Values{"startDateTime": {from.UTC().Format(time.RFC3339)}, "endDateTime": {to.UTC().Format(time.RFC3339)},
		"$select": {"id,type,start,end,originalStart,isAllDay,changeKey"}}
	raw, err := c.list(ctx, eventPath(masterID)+"/instances?"+q.Encode())
	if err != nil {
		return nil, err
	}
	return decodeAll[Event](raw)
}

// EventsByUID finds a calendar's events and series with an iCalendar UID.
func (c *Client) EventsByUID(ctx context.Context, calendarID, uid string) ([]Event, error) {
	q := url.Values{"$filter": {"iCalUId eq '" + strings.ReplaceAll(uid, "'", "''") + "'"},
		"$select": {"id,type,iCalUId,changeKey,isOrganizer,responseStatus"}}
	raw, err := c.list(ctx, "me/calendars/"+url.PathEscape(calendarID)+"/events?"+q.Encode())
	if err != nil {
		return nil, err
	}
	return decodeAll[Event](raw)
}

// CreateContact creates a contact in a folder (DefaultFolder for the
// default) and returns it.
func (c *Client) CreateContact(ctx context.Context, folder string, fields map[string]any) (Contact, error) {
	path := "me/contacts"
	if folder != DefaultFolder {
		path = "me/contactFolders/" + url.PathEscape(folder) + "/contacts"
	}
	raw, err := c.send(ctx, http.MethodPost, path, fields, "")
	if err != nil {
		return Contact{}, err
	}
	var gc Contact
	if err := json.Unmarshal(raw, &gc); err != nil {
		return Contact{}, fmt.Errorf("graph: created contact: %w", err)
	}
	return gc, nil
}

// UpdateContact changes a contact's fields, on its change key when
// ifMatch is set.
func (c *Client) UpdateContact(ctx context.Context, id string, fields map[string]any, ifMatch string) error {
	_, err := c.send(ctx, http.MethodPatch, contactPath(id), fields, ifMatch)
	return err
}

// DeleteContact deletes a contact, on its change key when ifMatch is set.
func (c *Client) DeleteContact(ctx context.Context, id, ifMatch string) error {
	_, err := c.send(ctx, http.MethodDelete, contactPath(id), nil, ifMatch)
	return err
}

// SetPhoto replaces a contact's photo; contentType is the image's media
// type.
func (c *Client) SetPhoto(ctx context.Context, contactID string, data []byte, contentType string) error {
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(contentType, "image/") {
		return fmt.Errorf("%w: a photo of type %s", ErrUnsupported, contentType)
	}
	_, _, err := c.Do(ctx, http.MethodPut, contactPath(contactID)+"/photo/$value", data,
		map[string]string{"Content-Type": contentType})
	return err
}

// ChangeKey reads an event's ETag, to check it before writes that cannot
// carry it.
func (c *Client) ChangeKey(ctx context.Context, id string) (string, error) {
	var e Event
	err := c.getJSON(ctx, eventPath(id)+"?$select=id", &e)
	return e.ETag, err
}

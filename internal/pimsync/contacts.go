package pimsync

import (
	"encoding/json/v2"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
	"github.com/frostyard/frostmail/internal/store"
	"github.com/frostyard/frostmail/internal/vcardx"
)

// ContactIndexOf is the index of a parsed vCard, as sync and the people
// API's writes store it.
func ContactIndexOf(card *vcardx.Card) store.ContactIndex {
	c := store.ContactIndex{DisplayName: card.DisplayName(), SortKey: card.SortKey(),
		GivenName: card.GivenName, FamilyName: card.FamilyName, Organization: card.Organization}
	for _, email := range card.Emails {
		c.Emails = append(c.Emails, store.ContactEmail{Email: email.Value, Label: email.Label})
	}
	if len(card.Photo.Data) > 0 {
		c.Photo, c.PhotoType = card.Photo.Data, card.Photo.Type
	}
	return c
}

// ContactPayload is a contact edit's put payload (ADR-0030; docs/design/
// pim.md, Editing): the changes in order and, by property, the lines the
// source had before the first one; or Whole, a source written whole (a
// restore), which cannot be applied again.
type ContactPayload struct {
	Changes []vcardx.Fields     `json:"vcard,omitzero"`
	Base    map[string][]string `json:"base,omitzero"`
	Whole   bool                `json:"whole,omitzero"`
	// Name is a delete's contact, for the app should the server refuse it.
	Name string `json:"name,omitzero"`
}

// ParseContactPayload reads a put's payload; ok is false for a put that is
// no contact edit (a creation, a task, an answer).
func ParseContactPayload(payload string) (ContactPayload, bool) {
	var p ContactPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil || len(p.Changes) == 0 && !p.Whole {
		return ContactPayload{}, false
	}
	return p, true
}

// Encode is the payload as stored.
func (p ContactPayload) Encode() (string, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("contact payload: %w", err)
	}
	return string(b), nil
}

// Extend adds a change made on src, recording the base of the properties
// it touches that no earlier change touched.
func (p ContactPayload) Extend(src []byte, f vcardx.Fields) (ContactPayload, error) {
	if p.Whole {
		return p, nil
	}
	lines, err := cardLines(src, touched(f))
	if err != nil {
		return p, err
	}
	out := ContactPayload{Changes: append(slices.Clone(p.Changes), f), Base: maps.Clone(p.Base)}
	if out.Base == nil {
		out.Base = map[string][]string{}
	}
	for name, l := range lines {
		if _, ok := out.Base[name]; !ok {
			out.Base[name] = l
		}
	}
	return out, nil
}

// Fits reports whether another client left every property the changes
// touch as it was: the server's lines equal the base.
func (p ContactPayload) Fits(server []byte) bool {
	if p.Whole {
		return false
	}
	lines, err := cardLines(server, slices.Collect(maps.Keys(p.Base)))
	if err != nil {
		return false
	}
	for name, base := range p.Base {
		if !slices.Equal(lines[name], base) {
			return false
		}
	}
	return true
}

// Apply makes the changes again, in order, on the server's source.
func (p ContactPayload) Apply(server []byte, now time.Time) ([]byte, error) {
	out := server
	for _, f := range p.Changes {
		var err error
		if out, err = vcardx.Patch(out, f, now); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// touched are the properties a change rewrites. A list change counts the
// group labels too: another client's relabel is a change to the list.
func touched(f vcardx.Fields) []string {
	var out []string
	add := func(set bool, names ...string) {
		if set {
			out = append(out, names...)
		}
	}
	add(f.GivenName != nil || f.FamilyName != nil, "N", "FN")
	add(f.Organization != nil, "ORG", "FN")
	add(f.Nickname != nil, "NICKNAME")
	add(f.Title != nil, "TITLE")
	add(f.Emails != nil, "EMAIL")
	add(f.Phones != nil, "TEL")
	add(f.Addresses != nil, "ADR")
	add(f.URLs != nil, "URL")
	add(f.Emails != nil || f.Phones != nil || f.Addresses != nil || f.URLs != nil, "X-ABLABEL")
	add(f.Birthday != nil, "BDAY")
	add(f.Note != nil, "NOTE")
	add(f.Photo != nil, "PHOTO")
	slices.Sort(out)
	return slices.Compact(out)
}

// cardLines are the unfolded lines of the named properties in the first
// VCARD, by name, in source order; a name without lines maps to nil.
func cardLines(src []byte, names []string) (map[string][]string, error) {
	components, err := contentline.Parse(src)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, name := range names {
		out[name] = nil
	}
	for _, c := range components {
		if c.Name != "VCARD" {
			continue
		}
		for _, p := range c.Props {
			if _, ok := out[p.Name]; ok {
				out[p.Name] = append(out[p.Name], strings.ReplaceAll(strings.TrimSuffix(p.Encode("\n"), "\n"), "\n ", ""))
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("no vcard component")
}

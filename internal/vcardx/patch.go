package vcardx

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
)

// Fields is a change to a contact (people.update's ContactFields, ADR-0030):
// nil fields keep their values, and a list replaces the whole list.
type Fields struct {
	GivenName    *string
	FamilyName   *string
	Nickname     *string
	Organization *string
	Title        *string
	Emails       *[]Labeled // Pref is ignored: a rewritten line keeps its own
	Phones       *[]Labeled
	Addresses    *[]Address
	URLs         *[]Labeled
	Birthday     *string // YYYY-MM-DD, --MM-DD, or "" to remove BDAY
	Note         *string // "" removes NOTE
	Photo        *Photo  // a Photo without Data removes PHOTO
}

// Patch applies f to the first VCARD of src, rewriting only the lines it
// touches and leaving every other byte as the server sent it (ADR-0018,
// ADR-0030).
func Patch(src []byte, f Fields, now time.Time) ([]byte, error) {
	card, err := patchCard(src)
	if err != nil {
		return nil, err
	}
	if f == (Fields{}) {
		return src, nil
	}
	v4 := patchProp(card, "VERSION").Value == "4.0"
	eol := contentline.LineEnding(src)
	props, err := patchProperties(card, f, v4)
	if err != nil {
		return nil, err
	}
	if rev := card.Prop("REV"); rev != nil {
		p := *rev
		layout := "2006-01-02T15:04:05Z"
		if v4 {
			layout = "20060102T150405Z"
		}
		p.Value = now.UTC().Format(layout)
		props = append(props, p)
	}
	at := patchEndStart(src, card.End)
	var edits []contentline.Edit
	for _, p := range props {
		edits = append(edits, patchPropertyEdits(card, p, at, eol)...)
	}
	lists, err := listEdits(src, card, f, v4, eol)
	if err != nil {
		return nil, fmt.Errorf("patch vcard: %w", err)
	}
	edits = append(edits, lists...)
	out, err := contentline.Apply(src, edits...)
	if err != nil {
		return nil, fmt.Errorf("patch vcard: %w", err)
	}
	return out, nil
}

func patchCard(src []byte) (*contentline.Component, error) {
	components, err := contentline.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("patch vcard: %w", err)
	}
	for _, card := range components {
		if card.Name == "VCARD" {
			return card, nil
		}
	}
	return nil, fmt.Errorf("patch vcard: no vcard component")
}

// patchProp copies the first property, or prepares a parameterless insertion.
func patchProp(card *contentline.Component, name string) contentline.Prop {
	if p := card.Prop(name); p != nil {
		return *p
	}
	return contentline.Prop{Name: name}
}

func patchProperties(card *contentline.Component, f Fields, v4 bool) ([]contentline.Prop, error) {
	props := patchNames(card, f)
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"NICKNAME", f.Nickname}, {"ORG", f.Organization},
		{"TITLE", f.Title}, {"BDAY", f.Birthday}, {"NOTE", f.Note},
	} {
		if field.value == nil {
			continue
		}
		p := patchProp(card, field.name)
		switch field.name {
		case "ORG":
			fields := p.Fields()
			fields[0] = *field.value
			p.Value = ""
			if strings.Join(fields, "") != "" {
				p.Value = contentline.JoinFields(fields...)
			}
		case "BDAY":
			var err error
			p, err = patchBirthday(p, *field.value, v4)
			if err != nil {
				return nil, err
			}
		default:
			p.Value = contentline.EscapeText(*field.value)
		}
		props = append(props, p)
	}
	if f.Photo != nil {
		p, err := patchPhoto(patchProp(card, "PHOTO"), *f.Photo, v4)
		if err != nil {
			return nil, err
		}
		props = append(props, p)
	}
	return props, nil
}

func patchNames(card *contentline.Component, f Fields) []contentline.Prop {
	if f.GivenName == nil && f.FamilyName == nil && f.Organization == nil {
		return nil
	}
	n := patchProp(card, "N")
	var names [5]string
	copy(names[:], n.Fields())
	org := patchProp(card, "ORG").Fields()[0]
	oldName := patchDerivedName(names[1], names[0], org)
	if f.GivenName != nil {
		names[1] = *f.GivenName
	}
	if f.FamilyName != nil {
		names[0] = *f.FamilyName
	}
	if f.Organization != nil {
		org = *f.Organization
	}
	var props []contentline.Prop
	if f.GivenName != nil || f.FamilyName != nil {
		n.Value = contentline.JoinFields(names[:]...)
		props = append(props, n)
	}
	newName := patchDerivedName(names[1], names[0], org)
	fn := patchProp(card, "FN")
	current := strings.TrimSpace(fn.Text())
	if newName != "" && current != newName && (card.Prop("FN") == nil || current == oldName) {
		fn.Value = contentline.EscapeText(newName)
		props = append(props, fn)
	}
	return props
}

func patchDerivedName(given, family, org string) string {
	if name := strings.TrimSpace(given + " " + family); name != "" {
		return name
	}
	return strings.TrimSpace(org)
}

func patchBirthday(p contentline.Prop, value string, v4 bool) (contentline.Prop, error) {
	if err := validatePatchBirthday(value); err != nil {
		return p, err
	}
	apple := false
	var params []contentline.Param
	for _, param := range p.Params {
		switch param.Name {
		case "X-APPLE-OMIT-YEAR":
			apple = true
		case "VALUE":
		default:
			params = append(params, param)
		}
	}
	p.Params, p.Value = params, value
	noYear := strings.HasPrefix(value, "--")
	switch {
	case noYear && apple:
		p.Params = append(p.Params, contentline.Param{Name: "X-APPLE-OMIT-YEAR", Values: []string{"1604"}})
		p.Value = "1604" + value[1:]
	case noYear && v4:
		p.Value = "--" + strings.ReplaceAll(value[2:], "-", "")
	case v4:
		p.Value = strings.ReplaceAll(value, "-", "")
	}
	return p, nil
}

func validatePatchBirthday(value string) error {
	if value == "" {
		return nil
	}
	date := value
	if strings.HasPrefix(value, "--") && len(value) == 7 {
		date = "2000" + value[1:]
	}
	if len(date) != 10 {
		return fmt.Errorf("patch vcard: invalid birthday %q", value)
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return fmt.Errorf("patch vcard: invalid birthday %q: %w", value, err)
	}
	return nil
}

func patchPhoto(p contentline.Prop, photo Photo, v4 bool) (contentline.Prop, error) {
	p.Params, p.Value = nil, ""
	if len(photo.Data) == 0 {
		return p, nil
	}
	if photo.Type != "image/jpeg" && photo.Type != "image/png" {
		return p, fmt.Errorf("patch vcard: unsupported photo type %q", photo.Type)
	}
	p.Value = base64.StdEncoding.EncodeToString(photo.Data)
	if v4 {
		p.Value = "data:" + photo.Type + ";base64," + p.Value
	} else {
		p.Params = []contentline.Param{
			{Name: "ENCODING", Values: []string{"b"}},
			{Name: "TYPE", Values: []string{strings.ToUpper(strings.TrimPrefix(photo.Type, "image/"))}},
		}
	}
	return p, nil
}

func patchPropertyEdits(card *contentline.Component, p contentline.Prop, at int, eol string) []contentline.Edit {
	text := ""
	if p.Value != "" {
		text = p.Encode(eol)
	}
	old := card.PropsNamed(p.Name)
	if len(old) == 0 {
		if text == "" {
			return nil
		}
		return []contentline.Edit{{Start: at, End: at, Text: text}}
	}
	edits := make([]contentline.Edit, 0, len(old))
	for _, prop := range old {
		edits = append(edits, contentline.Edit{Start: prop.Start, End: prop.End, Text: text})
		text = "" // Later occurrences are removed, including their folded lines.
	}
	return edits
}

// patchEndStart finds the END line's start, including any continuations.
func patchEndStart(src []byte, end int) int {
	for {
		lineEnd := end
		if lineEnd > 0 && src[lineEnd-1] == '\n' {
			lineEnd--
		}
		start := bytes.LastIndexByte(src[:lineEnd], '\n') + 1
		if src[start] != ' ' && src[start] != '\t' {
			return start
		}
		end = start
	}
}

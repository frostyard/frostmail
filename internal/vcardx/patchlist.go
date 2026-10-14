package vcardx

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
)

type listValue struct {
	label   string
	value   string
	address Address
}

type listField struct {
	name   string
	values []listValue
}

type listPatcher struct {
	card      *contentline.Component
	labels    map[string]string
	v4        bool
	eol       string
	nextGroup int
	edits     []contentline.Edit
	removed   map[int]bool
}

// listEdits returns the edits that make card's EMAIL, TEL, ADR and URL
// lines say f's lists, in a source whose lines end in eol; v4 is a vCard
// 4.0. It returns no edits when f sets no list.
func listEdits(src []byte, card *contentline.Component, f Fields, v4 bool, eol string) ([]contentline.Edit, error) {
	p := listPatcher{
		card: card, labels: groupLabels(card.Props), v4: v4, eol: eol,
		nextGroup: nextListGroup(card), removed: make(map[int]bool),
	}
	for _, field := range patchLists(f) {
		p.patchList(field, patchEndStart(src, card.End))
	}
	// Insertions must precede replacements/removals at the same offset.
	slices.SortStableFunc(p.edits, func(a, b contentline.Edit) int {
		if order := cmp.Compare(a.Start, b.Start); order != 0 {
			return order
		}
		return cmp.Compare(a.End, b.End)
	})
	return p.edits, nil
}

func patchLists(f Fields) []listField {
	var fields []listField
	for _, list := range []struct {
		name   string
		values *[]Labeled
	}{{"EMAIL", f.Emails}, {"TEL", f.Phones}} {
		if list.values != nil {
			fields = append(fields, labeledList(list.name, *list.values))
		}
	}
	if f.Addresses != nil {
		field := listField{name: "ADR"}
		for _, address := range *f.Addresses {
			field.values = append(field.values, listValue{label: address.Label, address: address})
		}
		fields = append(fields, field)
	}
	if f.URLs != nil {
		fields = append(fields, labeledList("URL", *f.URLs))
	}
	return fields
}

func labeledList(name string, values []Labeled) listField {
	field := listField{name: name}
	for _, value := range values {
		field.values = append(field.values, listValue{label: value.Label, value: value.Value})
	}
	return field
}

func nextListGroup(card *contentline.Component) int {
	next := 1
	for _, prop := range card.Props {
		group := strings.ToLower(prop.Group)
		if suffix, ok := strings.CutPrefix(group, "item"); ok {
			if number, err := strconv.Atoi(suffix); err == nil && number >= next {
				next = number + 1
			}
		}
	}
	return next
}

func (p *listPatcher) patchList(field listField, at int) {
	old := p.card.PropsNamed(field.name)
	for i, prop := range old {
		if i < len(field.values) {
			p.rewrite(prop, field.values[i])
		} else {
			p.remove(prop)
			p.removeGroup(prop.Group, false)
		}
	}
	if len(field.values) <= len(old) {
		return
	}
	if len(old) > 0 {
		last := old[len(old)-1]
		at = last.End
		for _, prop := range p.card.Props {
			if last.Group != "" && strings.EqualFold(prop.Group, last.Group) && strings.HasPrefix(prop.Name, "X-AB") {
				at = max(at, prop.End)
			}
		}
	}
	var added strings.Builder
	for _, value := range field.values[len(old):] {
		added.WriteString(p.newLine(field.name, value))
	}
	p.edits = append(p.edits, contentline.Edit{Start: at, End: at, Text: added.String()})
}

func (p *listPatcher) rewrite(old contentline.Prop, value listValue) {
	prop := old
	prop.Value = listPropertyValue(old, value, p.v4)
	if propertyLabel(old, p.labels) != value.label {
		prop.Params = withoutListLabels(prop.Params)
		label := p.labelLine(&prop, value.label)
		if label != "" {
			p.edits = append(p.edits, contentline.Edit{Start: old.End, End: old.End, Text: label})
		}
		p.removeGroup(old.Group, true)
	}
	sameParams := slices.EqualFunc(old.Params, prop.Params, func(a, b contentline.Param) bool {
		return a.Name == b.Name && slices.Equal(a.Values, b.Values)
	})
	if old.Group != prop.Group || old.Value != prop.Value || !sameParams {
		p.edits = append(p.edits, contentline.Edit{Start: old.Start, End: old.End, Text: prop.Encode(p.eol)})
	}
}

func (p *listPatcher) remove(prop contentline.Prop) {
	if !p.removed[prop.Start] {
		p.removed[prop.Start] = true
		p.edits = append(p.edits, contentline.Edit{Start: prop.Start, End: prop.End})
	}
}

func (p *listPatcher) removeGroup(group string, labelsOnly bool) {
	if group == "" {
		return
	}
	for _, prop := range p.card.Props {
		if labelsOnly && prop.Name != "X-ABLABEL" {
			continue
		}
		if strings.EqualFold(prop.Group, group) && strings.HasPrefix(prop.Name, "X-AB") {
			p.remove(prop)
		}
	}
}

func listPrefix(name string) string {
	switch name {
	case "EMAIL":
		return "mailto:"
	case "TEL":
		return "tel:"
	default:
		return ""
	}
}

func listPropertyValue(prop contentline.Prop, value listValue, v4 bool) string {
	if prop.Name == "ADR" {
		return listAddressValue(prop, value.address)
	}
	prefix := listPrefix(prop.Name)
	if readLabeled(prop, nil, prefix).Value == value.value {
		return prop.Value
	}
	text := strings.NewReplacer("\r", "", "\n", "").Replace(value.value)
	old := strings.TrimSpace(prop.Text())
	if v4 && prefix != "" && len(old) >= len(prefix) && strings.EqualFold(old[:len(prefix)], prefix) {
		text = old[:len(prefix)] + text
	}
	return text
}

func listAddressValue(prop contentline.Prop, address Address) string {
	old := readAddress(prop, nil)
	old.Label, address.Label = "", ""
	if old == address {
		return prop.Value
	}
	fields := [7]string{"", "", address.Street, address.Locality, address.Region, address.Postcode, address.Country}
	if old.Street == address.Street {
		copy(fields[:3], prop.Fields())
	}
	return contentline.JoinFields(fields[:]...)
}

func withoutListLabels(params []contentline.Param) []contentline.Param {
	var kept []contentline.Param
	for _, param := range params {
		if param.Name != "TYPE" {
			kept = append(kept, param)
			continue
		}
		var values []string
		for _, value := range (contentline.Prop{Params: []contentline.Param{param}}).ParamValues("TYPE") {
			switch strings.ToLower(value) {
			case "home", "work", "other", "cell", "mobile", "iphone", "fax", "pager", "main":
			default:
				values = append(values, value)
			}
		}
		if len(values) > 0 {
			param.Values = []string{strings.Join(values, ",")}
			kept = append(kept, param)
		}
	}
	return kept
}

func listLabelType(name, label string) string {
	switch label {
	case "home", "work", "other":
		return strings.ToUpper(label)
	}
	if name == "TEL" {
		switch label {
		case "mobile":
			return "CELL"
		case "main", "fax", "pager":
			return strings.ToUpper(label)
		}
	}
	return ""
}

func (p *listPatcher) labelLine(prop *contentline.Prop, label string) string {
	if label == "" {
		return ""
	}
	if kind := listLabelType(prop.Name, label); kind != "" {
		if p.v4 {
			kind = strings.ToLower(kind)
		}
		prop.Params = append(prop.Params, contentline.Param{Name: "TYPE", Values: []string{kind}})
		return ""
	}
	if prop.Group == "" {
		prop.Group = "item" + strconv.Itoa(p.nextGroup)
		p.nextGroup++
	}
	if label == "homepage" {
		label = "_$!<HomePage>!$_"
	}
	return (contentline.Prop{Group: prop.Group, Name: "X-ABLabel", Value: contentline.EscapeText(label)}).Encode(p.eol)
}

func (p *listPatcher) newLine(name string, value listValue) string {
	prop := contentline.Prop{Name: name}
	prop.Value = listPropertyValue(prop, value, false)
	if name == "ADR" {
		a := value.address
		prop.Value = contentline.JoinFields("", "", a.Street, a.Locality, a.Region, a.Postcode, a.Country)
	}
	if name == "EMAIL" && !p.v4 {
		prop.Params = []contentline.Param{{Name: "TYPE", Values: []string{"INTERNET"}}}
	}
	label := p.labelLine(&prop, value.label)
	return prop.Encode(p.eol) + label
}

// NewCard writes the vCard 3.0 people.create stores: a new contact with
// f's fields. It refuses what Patch refuses.
func NewCard(uid string, f Fields, now time.Time) ([]byte, error) {
	props, err := patchProperties(&contentline.Component{}, f, false)
	if err != nil {
		return nil, err
	}
	var out strings.Builder
	for _, prop := range newCardHeader(uid, f) {
		out.WriteString(prop.Encode("\r\n"))
	}
	for _, prop := range props {
		if prop.Value != "" && (prop.Name == "NICKNAME" || prop.Name == "ORG" || prop.Name == "TITLE") {
			out.WriteString(prop.Encode("\r\n"))
		}
	}
	p := listPatcher{eol: "\r\n", nextGroup: 1}
	for _, field := range patchLists(f) {
		for _, value := range field.values {
			out.WriteString(p.newLine(field.name, value))
		}
	}
	for _, prop := range props {
		if prop.Value != "" && (prop.Name == "BDAY" || prop.Name == "NOTE" || prop.Name == "PHOTO") {
			out.WriteString(prop.Encode("\r\n"))
		}
	}
	out.WriteString((contentline.Prop{Name: "REV", Value: now.UTC().Format("2006-01-02T15:04:05Z")}).Encode("\r\n"))
	out.WriteString((contentline.Prop{Name: "END", Value: "VCARD"}).Encode("\r\n"))
	return []byte(out.String()), nil
}

func newCardHeader(uid string, f Fields) []contentline.Prop {
	var given, family, org string
	if f.GivenName != nil {
		given = *f.GivenName
	}
	if f.FamilyName != nil {
		family = *f.FamilyName
	}
	if f.Organization != nil {
		org = *f.Organization
	}
	name := patchDerivedName(given, family, org)
	if name == "" && f.Emails != nil && len(*f.Emails) > 0 {
		name = (*f.Emails)[0].Value
	}
	return []contentline.Prop{
		{Name: "BEGIN", Value: "VCARD"},
		{Name: "VERSION", Value: "3.0"},
		{Name: "PRODID", Value: "-//Frostyard//Frostmail//EN"},
		{Name: "UID", Value: contentline.EscapeText(uid)},
		{Name: "FN", Value: contentline.EscapeText(name)},
		{Name: "N", Value: contentline.JoinFields(family, given, "", "", "")},
	}
}

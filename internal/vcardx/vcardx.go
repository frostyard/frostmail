// Package vcardx reads the fields Frostmail shows from vCards (RFC 2426
// version 3.0 and RFC 6350 version 4.0) over internal/contentline, and
// builds the vCard Add to Contacts creates (docs/design/pim.md).
package vcardx

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/frostyard/frostmail/internal/contentline"
)

// Card is what Frostmail reads from a vCard.
type Card struct {
	UID           string
	Kind          string // KIND (or X-ADDRESSBOOKSERVER-KIND), lowercased; "" means individual
	FormattedName string // FN
	FamilyName    string // N's fields
	GivenName     string
	MiddleName    string
	Prefix        string
	Suffix        string
	Nickname      string
	Organization  string // ORG's first field
	Department    string // ORG's other fields, joined by ", "
	Title         string
	Emails        []Labeled
	Phones        []Labeled
	Addresses     []Address
	URLs          []Labeled
	Birthday      string // YYYY-MM-DD, --MM-DD, or ""
	Note          string
	Photo         Photo
}

// Labeled is an email address, phone number or URL with its label.
type Labeled struct {
	Label string
	Value string
	Pref  bool
}

// Address is a postal address.
type Address struct {
	Label    string
	Street   string
	Locality string
	Region   string
	Postcode string
	Country  string
}

// Photo is a contact's photo: inline bytes, or a URI to fetch.
type Photo struct {
	Type string // the media type of Data, such as image/jpeg
	Data []byte
	URI  string
}

// Parse reads the first vCard in raw, decoding the fields Frostmail displays.
func Parse(raw []byte) (*Card, error) {
	components, err := contentline.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse vcard: %w", err)
	}
	for _, component := range components {
		if component.Name == "VCARD" {
			c := &Card{}
			labels := groupLabels(component.Props)
			for _, p := range component.Props {
				c.readProperty(p, labels)
			}
			return c, nil
		}
	}
	return nil, fmt.Errorf("parse vcard: no vcard component")
}

func (c *Card) readProperty(p contentline.Prop, labels map[string]string) {
	textFields := map[string]*string{
		"UID": &c.UID, "FN": &c.FormattedName,
		"NICKNAME": &c.Nickname, "TITLE": &c.Title,
	}
	if field, ok := textFields[p.Name]; ok {
		*field = strings.TrimSpace(p.Text())
		return
	}
	switch p.Name {
	case "NOTE":
		c.Note = p.Text()
	case "KIND", "X-ADDRESSBOOKSERVER-KIND":
		c.Kind = strings.ToLower(strings.TrimSpace(p.Text()))
	case "N":
		fields := trimmedFields(p, 5)
		c.FamilyName, c.GivenName, c.MiddleName = fields[0], fields[1], fields[2]
		c.Prefix, c.Suffix = fields[3], fields[4]
	case "ORG":
		fields := trimmedFields(p, 1)
		c.Organization = fields[0]
		c.Department = joinNonempty(fields[1:], ", ")
	case "EMAIL":
		c.Emails = append(c.Emails, readLabeled(p, labels, "mailto:"))
	case "TEL":
		c.Phones = append(c.Phones, readLabeled(p, labels, "tel:"))
	case "URL":
		c.URLs = append(c.URLs, readLabeled(p, labels, ""))
	case "ADR":
		c.Addresses = append(c.Addresses, readAddress(p, labels))
	case "BDAY":
		c.Birthday = readBirthday(p)
	case "PHOTO":
		if c.Photo.URI == "" && len(c.Photo.Data) == 0 {
			c.Photo = readPhoto(p)
		}
	}
}

func trimmedFields(p contentline.Prop, count int) []string {
	fields := p.Fields()
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	for len(fields) < count {
		fields = append(fields, "")
	}
	return fields
}

func joinNonempty(fields []string, separator string) string {
	var nonempty []string
	for _, field := range fields {
		if field != "" {
			nonempty = append(nonempty, field)
		}
	}
	return strings.Join(nonempty, separator)
}

func groupLabels(props []contentline.Prop) map[string]string {
	labels := make(map[string]string)
	for _, p := range props {
		if p.Name != "X-ABLABEL" || p.Group == "" {
			continue
		}
		group := strings.ToLower(p.Group)
		if _, exists := labels[group]; exists {
			continue
		}
		label := p.Text()
		if strings.HasPrefix(label, "_$!<") && strings.HasSuffix(label, ">!$_") {
			label = strings.ToLower(label[4 : len(label)-4])
			switch label {
			case "homefax", "workfax":
				label = "fax"
			case "iphone":
				label = "mobile"
			}
		}
		labels[group] = label
	}
	return labels
}

func propertyLabel(p contentline.Prop, labels map[string]string) string {
	if label, ok := labels[strings.ToLower(p.Group)]; ok {
		return label
	}
	if p.Name == "TEL" {
		if p.HasParam("TYPE", "fax") {
			return "fax"
		}
		for _, mobile := range []string{"cell", "mobile", "iphone"} {
			if p.HasParam("TYPE", mobile) {
				return "mobile"
			}
		}
		for _, label := range []string{"pager", "main"} {
			if p.HasParam("TYPE", label) {
				return label
			}
		}
	}
	for _, label := range []string{"home", "work", "other"} {
		if p.HasParam("TYPE", label) {
			return label
		}
	}
	return ""
}

func readLabeled(p contentline.Prop, labels map[string]string, prefix string) Labeled {
	value := strings.TrimSpace(p.Text())
	if len(value) >= len(prefix) && strings.EqualFold(value[:len(prefix)], prefix) {
		value = strings.TrimSpace(value[len(prefix):])
	}
	pref := p.HasParam("TYPE", "pref")
	for _, param := range p.Params {
		if strings.EqualFold(param.Name, "PREF") {
			pref = true
		}
	}
	return Labeled{Label: propertyLabel(p, labels), Value: value, Pref: pref}
}

func readAddress(p contentline.Prop, labels map[string]string) Address {
	fields := trimmedFields(p, 7)
	return Address{
		Label: propertyLabel(p, labels), Street: joinNonempty(fields[:3], "\n"),
		Locality: fields[3], Region: fields[4], Postcode: fields[5], Country: fields[6],
	}
}

func readBirthday(p contentline.Prop) string {
	value, _, _ := strings.Cut(strings.TrimSpace(p.Text()), "T")
	switch {
	case len(value) == 8 && !strings.HasPrefix(value, "--"):
		value = value[:4] + "-" + value[4:6] + "-" + value[6:]
	case len(value) == 6 && strings.HasPrefix(value, "--"):
		value = value[:4] + "-" + value[4:]
	}
	date := value
	if strings.HasPrefix(date, "--") {
		date = "2000" + date[1:]
	}
	if len(date) != 10 {
		return ""
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return ""
	}
	if len(value) == 10 && p.Param("X-APPLE-OMIT-YEAR") == value[:4] {
		return "--" + value[5:]
	}
	return value
}

func readPhoto(p contentline.Prop) Photo {
	value := strings.TrimSpace(p.Text())
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return Photo{URI: value}
	}
	mediaType := ""
	if strings.HasPrefix(lower, "data:") {
		index := strings.Index(lower, ";base64,")
		if index < 0 {
			return Photo{}
		}
		mediaType, _, _ = strings.Cut(lower[5:index], ";")
		value = value[index+8:]
	} else if p.HasParam("ENCODING", "b") || p.HasParam("ENCODING", "base64") {
		mediaType = strings.ToLower(p.Param("TYPE"))
		if mediaType != "" && !strings.Contains(mediaType, "/") {
			mediaType = "image/" + mediaType
		}
	} else {
		return Photo{}
	}
	value = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value)
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(data) == 0 {
		return Photo{}
	}
	if mediaType == "" {
		mediaType = http.DetectContentType(data)
	}
	return Photo{Type: mediaType, Data: data}
}

// DisplayName returns the formatted name, or the first available name or email.
func (c *Card) DisplayName() string {
	for _, name := range []string{strings.TrimSpace(c.FormattedName),
		strings.TrimSpace(c.GivenName + " " + c.FamilyName), c.Nickname, c.Organization} {
		if name != "" {
			return name
		}
	}
	if len(c.Emails) > 0 {
		return c.Emails[0].Value
	}
	return ""
}

// SortKey returns a lowercase family-first name, falling back to DisplayName.
func (c *Card) SortKey() string {
	if c.FamilyName != "" {
		return strings.ToLower(strings.TrimSpace(c.FamilyName + " " + c.GivenName))
	}
	return strings.ToLower(c.DisplayName())
}

// New builds a folded vCard 3.0 for Add to Contacts with a name and email.
func New(uid, name, email string) []byte {
	name = strings.TrimSpace(name)
	words := strings.Fields(name)
	family, given := "", ""
	if len(words) == 1 {
		given = words[0]
	} else if len(words) > 1 {
		family = words[len(words)-1]
		given = strings.Join(words[:len(words)-1], " ")
	}
	if name == "" {
		name = email
	}
	lines := []string{
		"BEGIN:VCARD", "VERSION:3.0", "PRODID:-//Frostyard//Frostmail//EN",
		"UID:" + uid, "FN:" + contentline.EscapeText(name),
		"N:" + contentline.JoinFields(family, given, "", "", ""),
		"EMAIL;TYPE=INTERNET:" + email, "END:VCARD",
	}
	var out strings.Builder
	for _, line := range lines {
		out.WriteString(contentline.Fold(line, "\r\n"))
	}
	return []byte(out.String())
}

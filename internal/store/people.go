package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode"
)

// PersonRow is a person as the People list and contact cards show them.
type PersonRow struct {
	ID           int64
	DisplayName  string
	SortKey      string
	Organization string
	Email        string // the first email of the person's first contact, or ""
	HasPhoto     bool
	ContactIDs   []int64 // the object IDs of the person's contacts, ascending
}

const eligibleContactJoins = ` JOIN objects o ON o.id = c.object_id
 JOIN collections col ON col.id = o.collection_id
 JOIN account_services s ON s.account_id = col.account_id AND s.service = 'contacts'
 WHERE col.kind = 'addressbook' AND col.enabled = 1 AND s.enabled = 1`

// RelinkPeople joins eligible contacts transitively by email, retaining unchanged rows.
// Ineligible contacts lose their person; no events are emitted.
func (t *Tx) RelinkPeople(ctx context.Context) error {
	contacts, err := t.peopleContacts(ctx)
	if err != nil {
		return err
	}
	parents := make(map[int64]int64, len(contacts))
	for _, c := range contacts {
		parents[c.ID] = c.ID
	}
	if err := t.joinContactEmails(ctx, parents); err != nil {
		return err
	}
	persons := map[int64]PersonRow{}
	for _, c := range contacts {
		id := personRoot(parents, c.ID)
		p := persons[id]
		p.ID = id
		if p.DisplayName == "" && c.DisplayName != "" {
			p.DisplayName, p.SortKey = c.DisplayName, c.SortKey
			if p.SortKey == "" {
				p.SortKey = strings.ToLower(p.DisplayName)
			}
		}
		if p.Organization == "" {
			p.Organization = c.Organization
		}
		persons[id] = p
	}
	for _, p := range persons {
		_, err := t.ExecContext(ctx, `INSERT INTO people (id, display_name, sort_key, organization)
 VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET display_name = excluded.display_name,
 sort_key = excluded.sort_key, organization = excluded.organization
 WHERE display_name != excluded.display_name OR sort_key != excluded.sort_key
 OR organization != excluded.organization`, p.ID, p.DisplayName, p.SortKey, p.Organization)
		if err != nil {
			return fmt.Errorf("relink person: %w", err)
		}
	}
	for _, c := range contacts {
		id := personRoot(parents, c.ID)
		if _, err := t.ExecContext(ctx, "UPDATE contacts SET person_id = ? WHERE object_id = ? AND person_id IS NOT ?", id, c.ID, id); err != nil {
			return fmt.Errorf("link contact: %w", err)
		}
	}
	if _, err := t.ExecContext(ctx, `UPDATE contacts SET person_id = NULL WHERE person_id IS NOT NULL
 AND object_id NOT IN (SELECT c.object_id FROM contacts c`+eligibleContactJoins+`)`); err != nil {
		return fmt.Errorf("unlink contact: %w", err)
	}
	if _, err := t.ExecContext(ctx, "DELETE FROM people WHERE id NOT IN (SELECT person_id FROM contacts WHERE person_id IS NOT NULL)"); err != nil {
		return fmt.Errorf("remove obsolete people: %w", err)
	}
	return nil
}

func (t *Tx) peopleContacts(ctx context.Context) ([]PersonRow, error) {
	rows, err := t.QueryContext(ctx, "SELECT c.object_id, c.display_name, c.sort_key, c.organization FROM contacts c"+eligibleContactJoins+" ORDER BY c.object_id")
	if err != nil {
		return nil, fmt.Errorf("eligible contacts: %w", err)
	}
	defer rows.Close()
	var out []PersonRow
	for rows.Next() {
		var c PersonRow
		if err := rows.Scan(&c.ID, &c.DisplayName, &c.SortKey, &c.Organization); err != nil {
			return nil, fmt.Errorf("eligible contact: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func personRoot(parents map[int64]int64, id int64) int64 {
	root := id
	for parents[root] != root {
		root = parents[root]
	}
	for id != root {
		next := parents[id]
		parents[id] = root
		id = next
	}
	return root
}

func (t *Tx) joinContactEmails(ctx context.Context, parents map[int64]int64) error {
	rows, err := t.QueryContext(ctx, "SELECT c.object_id, ce.email FROM contacts c JOIN contact_emails ce ON ce.contact_id = c.object_id"+eligibleContactJoins)
	if err != nil {
		return fmt.Errorf("eligible emails: %w", err)
	}
	defer rows.Close()
	owners := map[string]int64{}
	for rows.Next() {
		var id int64
		var email string
		if err := rows.Scan(&id, &email); err != nil {
			return fmt.Errorf("eligible email: %w", err)
		}
		if other, ok := owners[email]; ok {
			a, b := personRoot(parents, id), personRoot(parents, other)
			parents[max(a, b)] = min(a, b)
		} else {
			owners[email] = id
		}
	}
	return rows.Err()
}

func peopleWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func matchesPeopleWords(words, query []string) bool {
	for _, q := range query {
		found := false
		for _, w := range words {
			if strings.HasPrefix(w, q) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// People returns people ordered by sort key and ID, matching every query word
// against word prefixes in their name, organization and contacts' emails.
func (d *DB) People(ctx context.Context, query string) ([]PersonRow, error) {
	return d.readPeople(ctx, "", nil, strings.Fields(strings.ToLower(query)))
}

func (d *DB) readPeople(ctx context.Context, where string, args []any, query []string) ([]PersonRow, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT p.id, p.display_name, p.sort_key, p.organization,
 c.object_id, COALESCE(length(c.photo), 0) > 0, COALESCE(ce.email, '')
 FROM people p JOIN contacts c ON c.person_id = p.id
 LEFT JOIN contact_emails ce ON ce.contact_id = c.object_id `+where+`
 ORDER BY p.sort_key, p.id, c.object_id, ce.position`, args...)
	if err != nil {
		return nil, fmt.Errorf("read people: %w", err)
	}
	defer rows.Close()
	var out []PersonRow
	var current PersonRow
	var words []string
	flush := func() {
		if current.ID != 0 && matchesPeopleWords(words, query) {
			out = append(out, current)
		}
	}
	for rows.Next() {
		var p PersonRow
		var contactID int64
		if err := rows.Scan(&p.ID, &p.DisplayName, &p.SortKey, &p.Organization, &contactID, &p.HasPhoto, &p.Email); err != nil {
			return nil, fmt.Errorf("read person: %w", err)
		}
		if p.ID != current.ID {
			flush()
			current = p
			current.ContactIDs = []int64{}
			words = peopleWords(p.DisplayName + " " + p.Organization)
		}
		if len(current.ContactIDs) == 0 || current.ContactIDs[len(current.ContactIDs)-1] != contactID {
			current.ContactIDs = append(current.ContactIDs, contactID)
		}
		current.HasPhoto = current.HasPhoto || p.HasPhoto
		words = append(words, peopleWords(p.Email)...)
	}
	flush()
	return out, rows.Err()
}

// Person returns a person's list fields and ascending contact IDs, or ErrNotFound.
func (d *DB) Person(ctx context.Context, id int64) (PersonRow, error) {
	rows, err := d.readPeople(ctx, "WHERE p.id = ?", []any{id}, nil)
	if err != nil {
		return PersonRow{}, err
	}
	if len(rows) == 0 {
		return PersonRow{}, ErrNotFound
	}
	return rows[0], nil
}

// PersonByEmail returns the person of an eligible contact with the normalized email.
func (d *DB) PersonByEmail(ctx context.Context, email string) (PersonRow, error) {
	var id int64
	err := d.db.QueryRowContext(ctx, `SELECT c.person_id FROM contacts c
 JOIN contact_emails ce ON ce.contact_id = c.object_id`+eligibleContactJoins+`
 AND ce.email = ? AND c.person_id IS NOT NULL ORDER BY c.object_id LIMIT 1`, strings.ToLower(strings.TrimSpace(email))).Scan(&id)
	if err == sql.ErrNoRows {
		return PersonRow{}, ErrNotFound
	}
	if err != nil {
		return PersonRow{}, fmt.Errorf("person by email: %w", err)
	}
	return d.Person(ctx, id)
}

// PersonPhoto returns the first photo in ascending contact order, or ErrNotFound.
func (d *DB) PersonPhoto(ctx context.Context, id int64) ([]byte, string, error) {
	var data []byte
	var typ string
	err := d.db.QueryRowContext(ctx, `SELECT c.photo, c.photo_type FROM contacts c
 JOIN people p ON p.id = c.person_id WHERE p.id = ? AND length(c.photo) > 0
 ORDER BY c.object_id LIMIT 1`, id).Scan(&data, &typ)
	if err == sql.ErrNoRows {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("person photo: %w", err)
	}
	return data, typ, nil
}

// SuggestContacts returns distinct eligible emails matching an email or name-word
// prefix, ranked by mail frequency, recency, display name and email.
func (d *DB) SuggestContacts(ctx context.Context, prefix string, limit int) ([]Address, error) {
	if prefix == "" {
		return nil, nil
	}
	prefix = strings.ToLower(prefix)
	if limit <= 0 {
		limit = 10
	}
	rows, err := d.db.QueryContext(ctx, `SELECT DISTINCT ce.email, p.display_name,
 COALESCE(a.count, 0), a.last_seen FROM contacts c
 JOIN contact_emails ce ON ce.contact_id = c.object_id
 JOIN people p ON p.id = c.person_id LEFT JOIN addresses a ON a.address = ce.email`+eligibleContactJoins+`
 ORDER BY COALESCE(a.count, 0) DESC, a.last_seen DESC, p.display_name, ce.email`)
	if err != nil {
		return nil, fmt.Errorf("suggest contacts: %w", err)
	}
	defer rows.Close()
	var out []Address
	for rows.Next() {
		var a Address
		var count int64
		var seen sql.NullString
		if err := rows.Scan(&a.Addr, &a.Name, &count, &seen); err != nil {
			return nil, fmt.Errorf("suggest contact: %w", err)
		}
		if strings.HasPrefix(a.Addr, prefix) || matchesPeopleWords(peopleWords(a.Name), []string{prefix}) {
			out = append(out, a)
			if len(out) == limit {
				break
			}
		}
	}
	return out, rows.Err()
}

// MessagesWithAddress uses FTS candidates and exact address checks to return
// at most limit nondeleted messages in view order.
func (d *DB) MessagesWithAddress(ctx context.Context, email string, limit int) ([]int64, error) {
	if limit <= 0 || len(peopleWords(email)) == 0 {
		return nil, nil
	}
	email = strings.ToLower(email)
	phrase := "{from_text to_text} : \"" + strings.ReplaceAll(email, "\"", "\"\"") + "\""
	rows, err := d.db.QueryContext(ctx, `SELECT m.id FROM messages m
 WHERE m.id IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?)
 AND m.deleted = 0 AND (lower(m.from_addr) = ?
 OR EXISTS (SELECT 1 FROM json_each(m.to_json) WHERE lower(json_extract(value, '$.addr')) = ?)
 OR EXISTS (SELECT 1 FROM json_each(m.cc_json) WHERE lower(json_extract(value, '$.addr')) = ?))
 ORDER BY `+viewOrder+` LIMIT ?`, phrase, email, email, email, limit)
	if err != nil {
		return nil, fmt.Errorf("messages with address: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("message with address: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SeenName returns the mail address index's name, or an empty string without a row.
func (d *DB) SeenName(ctx context.Context, email string) (string, error) {
	var name string
	err := d.db.QueryRowContext(ctx, "SELECT name FROM addresses WHERE address = ?", strings.ToLower(email)).Scan(&name)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("seen name: %w", err)
	}
	return name, nil
}

// WritableAddressBooks returns enabled, writable books on writable accounts
// with contacts enabled, in account, default-first, position and ID order.
func (d *DB) WritableAddressBooks(ctx context.Context) ([]Collection, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT "+collectionColumns+` FROM collections
 WHERE kind = 'addressbook' AND enabled = 1 AND read_only = 0
 AND account_id IN (SELECT a.id FROM accounts a JOIN account_services s ON s.account_id = a.id
 WHERE a.read_only = 0 AND s.service = 'contacts' AND s.enabled = 1)
 ORDER BY account_id, is_default DESC, position, id`)
	if err != nil {
		return nil, fmt.Errorf("writable address books: %w", err)
	}
	defer rows.Close()
	var out []Collection
	for rows.Next() {
		c, err := readCollection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ContactsIn returns the object IDs of a collection's contacts.
func (d *DB) ContactsIn(ctx context.Context, collectionID int64) (map[int64]bool, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT c.object_id FROM contacts c
 JOIN objects o ON o.id = c.object_id WHERE o.collection_id = ?`, collectionID)
	if err != nil {
		return nil, fmt.Errorf("contacts in collection: %w", err)
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("contacts in collection: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

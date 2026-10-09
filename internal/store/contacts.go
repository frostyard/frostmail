package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ContactIndex is what the contacts and contact_emails tables hold for one
// vCard (internal/vcardx builds it).
type ContactIndex struct {
	DisplayName  string
	SortKey      string
	GivenName    string
	FamilyName   string
	Organization string
	Emails       []ContactEmail
	Photo        []byte
	PhotoType    string
}

// ContactEmail is one of a contact's email addresses.
type ContactEmail struct {
	Email string
	Label string
}

// IndexContact replaces contact fields and normalized emails, each once,
// retaining person_id.
func (t *Tx) IndexContact(ctx context.Context, objectID int64, c ContactIndex) error {
	_, err := t.ExecContext(ctx, `INSERT INTO contacts
 (object_id, display_name, sort_key, given_name, family_name, organization, photo, photo_type)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(object_id) DO UPDATE SET
 display_name = excluded.display_name, sort_key = excluded.sort_key, given_name = excluded.given_name,
 family_name = excluded.family_name, organization = excluded.organization,
 photo = excluded.photo, photo_type = excluded.photo_type`,
		objectID, c.DisplayName, c.SortKey, c.GivenName, c.FamilyName, c.Organization, c.Photo, c.PhotoType)
	if err != nil {
		return fmt.Errorf("index contact: %w", err)
	}
	if _, err := t.ExecContext(ctx, "DELETE FROM contact_emails WHERE contact_id = ?", objectID); err != nil {
		return fmt.Errorf("replace contact emails: %w", err)
	}
	position := 0
	seen := map[string]bool{}
	for _, e := range c.Emails {
		email := strings.ToLower(strings.TrimSpace(e.Email))
		if email == "" || seen[email] {
			continue
		}
		seen[email] = true
		if _, err := t.ExecContext(ctx, "INSERT INTO contact_emails (contact_id, position, email, label) VALUES (?, ?, ?, ?)",
			objectID, position, email, e.Label); err != nil {
			return fmt.Errorf("index contact email: %w", err)
		}
		position++
	}
	return nil
}

// RemoveContact removes contact fields and emails; an absent contact is harmless.
func (t *Tx) RemoveContact(ctx context.Context, objectID int64) error {
	if _, err := t.ExecContext(ctx, "DELETE FROM contacts WHERE object_id = ?", objectID); err != nil {
		return fmt.Errorf("remove contact: %w", err)
	}
	return nil
}

// Contact returns the stored fields and emails in position order, or ErrNotFound.
func (d *DB) Contact(ctx context.Context, objectID int64) (ContactIndex, error) {
	var c ContactIndex
	err := d.db.QueryRowContext(ctx, `SELECT display_name, sort_key, given_name, family_name,
 organization, photo, photo_type FROM contacts WHERE object_id = ?`, objectID).
		Scan(&c.DisplayName, &c.SortKey, &c.GivenName, &c.FamilyName, &c.Organization, &c.Photo, &c.PhotoType)
	if err == sql.ErrNoRows {
		return c, ErrNotFound
	}
	if err != nil {
		return c, fmt.Errorf("contact: %w", err)
	}
	rows, err := d.db.QueryContext(ctx, "SELECT email, label FROM contact_emails WHERE contact_id = ? ORDER BY position", objectID)
	if err != nil {
		return c, fmt.Errorf("contact emails: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e ContactEmail
		if err := rows.Scan(&e.Email, &e.Label); err != nil {
			return c, fmt.Errorf("contact emails: %w", err)
		}
		c.Emails = append(c.Emails, e)
	}
	return c, rows.Err()
}

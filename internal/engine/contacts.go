package engine

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/store"
	"github.com/frostyard/frostmail/internal/vcardx"
)

// The people domain's writes (ADR-0030, ADR-0032; docs/design/pim.md,
// Editing): each patches or builds the vCard, stores it and its index,
// keeps a restore, and queues the change for sync, in one transaction.

// maxPhoto is the largest photo people.setPhoto takes, in bytes.
const maxPhoto = 256 << 10

var emailAddress = regexp.MustCompile(`^[^@\s]+@[^@\s]+$`)

// Create adds a contact to an address book.
func (p people) Create(ctx context.Context, params *api.PeopleCreateParams) (*api.ContactWrite, error) {
	f, err := fieldsOf(params.Fields)
	if err != nil {
		return nil, err
	}
	book, err := p.addressBookFor(ctx, params.CollectionID)
	if err != nil {
		return nil, err
	}
	uid, err := p.newUID()
	if err != nil {
		return nil, err
	}
	raw, err := vcardx.NewCard(uid, f, p.DB.Now())
	if err != nil {
		return nil, api.InvalidParams("%v", err)
	}
	card, err := vcardx.Parse(raw)
	if err != nil {
		return nil, err
	}
	if card.DisplayName() == "" {
		return nil, api.InvalidParams("a contact needs a name, a company or an email address")
	}
	restoreID, err := newRestoreID()
	if err != nil {
		return nil, err
	}
	href := book.Href + uid + ".vcf"
	var id int64
	err = p.DB.Tx(ctx, func(tx *store.Tx) error {
		if id, err = tx.PutObject(ctx, store.Object{CollectionID: book.ID, Href: href, Kind: store.ObjectVCard,
			UID: uid, Raw: raw}); err != nil {
			return err
		}
		if err := indexCard(ctx, tx, id, card); err != nil {
			return err
		}
		if err := tx.RecordRestore(ctx, store.Restore{ID: restoreID, AccountID: book.AccountID,
			Objects: []store.RestoreObject{{CollectionID: book.ID, Href: href, UID: uid}}}); err != nil {
			return err
		}
		if _, err := tx.QueuePIMOp(ctx, store.PIMOp{AccountID: book.AccountID, CollectionID: book.ID, ObjectID: &id,
			Kind: "put", Href: href}); err != nil {
			return err
		}
		return tx.Emit(ctx, api.PeopleChanged{AccountID: book.AccountID})
	})
	if err != nil {
		return nil, err
	}
	return p.written(ctx, book.AccountID, id, id, restoreID)
}

// Update changes a contact's fields.
func (p people) Update(ctx context.Context, params *api.PeopleUpdateParams) (*api.ContactWrite, error) {
	f, err := fieldsOf(params.Fields)
	if err != nil {
		return nil, err
	}
	return p.edit(ctx, params.ID, f)
}

// SetPhoto sets or removes a contact's photo.
func (p people) SetPhoto(ctx context.Context, params *api.PeopleSetPhotoParams) (*api.ContactWrite, error) {
	photo := vcardx.Photo{}
	if params.Photo != nil {
		if params.Photo.ContentType != "image/jpeg" && params.Photo.ContentType != "image/png" {
			return nil, api.InvalidParams("a photo must be JPEG or PNG, not %q", params.Photo.ContentType)
		}
		data, err := base64.StdEncoding.DecodeString(params.Photo.Data)
		if err != nil || len(data) == 0 {
			return nil, api.InvalidParams("the photo is not base64")
		}
		if len(data) > maxPhoto {
			return nil, api.InvalidParams("a photo must be at most 256 KB, not %d bytes", len(data))
		}
		photo = vcardx.Photo{Type: params.Photo.ContentType, Data: data}
	}
	return p.edit(ctx, params.ID, vcardx.Fields{Photo: &photo})
}

// edit patches a contact, and queues the put or adds the change to the
// waiting one, with the base it was made on (ADR-0030). A contact a sync
// pass replaced since it was read is read again, up to writeAttempts times.
func (p people) edit(ctx context.Context, id int64, f vcardx.Fields) (*api.ContactWrite, error) {
	return retryWrite("the contact", func() (*api.ContactWrite, error) {
		w, err := p.prepareEdit(ctx, id, f)
		if err != nil {
			return nil, err
		}
		return p.writeEdit(ctx, w)
	})
}

// contactWrite is an edit or a deletion of a contact, made from the object
// as it was read.
type contactWrite struct {
	obj       store.Object
	col       store.Collection
	fields    vcardx.Fields // an edit's change
	raw       []byte        // an edit's patched source
	card      *vcardx.Card  // an edit's patched card
	stay      int64         // a deletion's other contact of the person
	restoreID string
}

// prepareEdit reads the contact and patches it, for writeEdit to store.
func (p people) prepareEdit(ctx context.Context, id int64, f vcardx.Fields) (*contactWrite, error) {
	obj, col, err := p.writableContact(ctx, id)
	if err != nil {
		return nil, err
	}
	raw, err := vcardx.Patch(obj.Raw, f, p.DB.Now())
	if err != nil {
		return nil, api.InvalidParams("%v", err)
	}
	card, err := vcardx.Parse(raw)
	if err != nil {
		return nil, err
	}
	restoreID, err := newRestoreID()
	if err != nil {
		return nil, err
	}
	return &contactWrite{obj: obj, col: col, fields: f, raw: raw, card: card, restoreID: restoreID}, nil
}

// writeEdit stores a prepared edit. The contact is read again in the
// transaction, so its put is queued on the ETag a pass wrote last; it
// returns errObjectChanged when the source is not the one that was
// patched.
func (p people) writeEdit(ctx context.Context, w *contactWrite) (*api.ContactWrite, error) {
	err := p.DB.Tx(ctx, func(tx *store.Tx) error {
		obj, err := currentObject(ctx, tx, w.obj)
		if err != nil {
			return err
		}
		if err := tx.RecordRestore(ctx, store.Restore{ID: w.restoreID, AccountID: w.col.AccountID,
			Objects: []store.RestoreObject{{CollectionID: w.col.ID, Href: obj.Href, UID: obj.UID, Raw: obj.Raw}}}); err != nil {
			return err
		}
		if err := queueEdit(ctx, tx, obj, w.col, w.fields); err != nil {
			return err
		}
		next := obj
		next.Raw = w.raw
		if _, err := tx.PutObject(ctx, next); err != nil {
			return err
		}
		if err := indexCard(ctx, tx, obj.ID, w.card); err != nil {
			return err
		}
		return tx.Emit(ctx, api.PeopleChanged{AccountID: w.col.AccountID})
	})
	if err != nil {
		return nil, err
	}
	return p.written(ctx, w.col.AccountID, w.obj.ID, w.obj.ID, w.restoreID)
}

// queueEdit queues an edit's put, or adds it to the put already waiting.
func queueEdit(ctx context.Context, tx *store.Tx, obj store.Object, col store.Collection, f vcardx.Fields) error {
	op, waiting, err := tx.QueuedPut(ctx, obj.ID)
	if err != nil {
		return err
	}
	payload := pimsync.ContactPayload{}
	if waiting {
		if payload, waiting = pimsync.ParseContactPayload(op.Payload); !waiting && op.IfMatch == "" {
			return nil // a creation writes the source as it is when it runs
		}
	}
	if payload, err = payload.Extend(obj.Raw, f); err != nil {
		return err
	}
	encoded, err := payload.Encode()
	if err != nil {
		return err
	}
	if op.ID != 0 {
		return tx.SetPIMOpPayload(ctx, op.ID, encoded)
	}
	_, err = tx.QueuePIMOp(ctx, store.PIMOp{AccountID: col.AccountID, CollectionID: col.ID, ObjectID: &obj.ID,
		Kind: "put", Href: obj.Href, IfMatch: obj.ETag, Payload: encoded})
	return err
}

// Delete deletes a contact from its address book and the server. A contact
// a sync pass replaced since it was read is read again, up to writeAttempts
// times.
func (p people) Delete(ctx context.Context, params *api.PeopleDeleteParams) (*api.ContactWrite, error) {
	return retryWrite("the contact", func() (*api.ContactWrite, error) {
		w, err := p.prepareDelete(ctx, params)
		if err != nil {
			return nil, err
		}
		return p.writeDelete(ctx, w)
	})
}

// prepareDelete reads the contact, for writeDelete to delete.
func (p people) prepareDelete(ctx context.Context, params *api.PeopleDeleteParams) (*contactWrite, error) {
	obj, col, err := p.writableContact(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	restoreID, err := newRestoreID()
	if err != nil {
		return nil, err
	}
	return &contactWrite{obj: obj, col: col, stay: p.otherContact(ctx, obj.ID), restoreID: restoreID}, nil
}

// writeDelete deletes a prepared contact. The contact is read again in the
// transaction, so its delete is queued on the ETag a pass wrote last; it
// returns errObjectChanged when the source is not the one that was read.
func (p people) writeDelete(ctx context.Context, w *contactWrite) (*api.ContactWrite, error) {
	err := p.DB.Tx(ctx, func(tx *store.Tx) error {
		obj, err := currentObject(ctx, tx, w.obj)
		if err != nil {
			return err
		}
		if err := tx.RecordRestore(ctx, store.Restore{ID: w.restoreID, AccountID: w.col.AccountID,
			Objects: []store.RestoreObject{{CollectionID: w.col.ID, Href: obj.Href, UID: obj.UID, Raw: obj.Raw}}}); err != nil {
			return err
		}
		if err := deleteContact(ctx, tx, obj, w.col); err != nil {
			return err
		}
		return tx.Emit(ctx, api.PeopleChanged{AccountID: w.col.AccountID})
	})
	if err != nil {
		return nil, err
	}
	return p.written(ctx, w.col.AccountID, w.obj.ID, w.stay, w.restoreID)
}

// deleteContact removes a contact here and queues its delete; one the
// server never had is only dropped, with its waiting changes.
func deleteContact(ctx context.Context, tx *store.Tx, obj store.Object, col store.Collection) error {
	if obj.ETag == "" {
		if _, err := tx.DropPIMOps(ctx, obj.ID); err != nil {
			return err
		}
	} else {
		name := ""
		if card, err := vcardx.Parse(obj.Raw); err == nil {
			name = card.DisplayName()
		}
		payload, err := pimsync.ContactPayload{Name: name}.Encode()
		if err != nil {
			return err
		}
		if _, err := tx.QueuePIMOp(ctx, store.PIMOp{AccountID: col.AccountID, CollectionID: col.ID,
			Kind: "delete", Href: obj.Href, IfMatch: obj.ETag, Payload: payload}); err != nil {
			return err
		}
	}
	if _, err := tx.DeleteObjects(ctx, col.ID, []string{obj.Href}); err != nil {
		return err
	}
	return tx.RelinkPeople(ctx)
}

// Restore writes back what a contact write replaced (ADR-0032).
func (p people) Restore(ctx context.Context, params *api.PeopleRestoreParams) error {
	var accountID int64
	err := p.DB.Tx(ctx, func(tx *store.Tx) error {
		r, err := tx.TakeRestore(ctx, params.RestoreID)
		if errors.Is(err, store.ErrNotFound) {
			return api.NotFound("restore %s is gone or was never made", params.RestoreID)
		}
		if err != nil {
			return err
		}
		accountID = r.AccountID
		for _, o := range r.Objects {
			if err := p.restoreObject(ctx, tx, o); err != nil {
				return err
			}
		}
		if err := tx.RelinkPeople(ctx); err != nil {
			return err
		}
		return tx.Emit(ctx, api.PeopleChanged{AccountID: r.AccountID})
	})
	if err != nil {
		return err
	}
	if p.PIM != nil {
		p.PIM.Kick(accountID)
	}
	return nil
}

// restoreObject puts one kept source back: deleting what the write made,
// writing the old source whole over the current one, or creating it again.
func (p people) restoreObject(ctx context.Context, tx *store.Tx, o store.RestoreObject) error {
	col, err := p.DB.GetCollection(ctx, o.CollectionID)
	if err != nil {
		return apiError(err, fmt.Sprintf("address book %d", o.CollectionID))
	}
	if err := p.writable(ctx, col); err != nil {
		return err
	}
	current, err := p.DB.ObjectByHref(ctx, col.ID, o.Href)
	exists := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if o.Raw == nil {
		if !exists {
			return nil
		}
		return deleteContact(ctx, tx, current, col)
	}
	card, err := vcardx.Parse(o.Raw)
	if err != nil {
		return err
	}
	next := store.Object{CollectionID: col.ID, Href: o.Href, Kind: store.ObjectVCard, UID: o.UID, Raw: o.Raw}
	switch {
	case exists:
		next.ETag = current.ETag
	default:
		// A delete not yet written is cancelled, and the server's object
		// stays; one already written is created again under a new name,
		// since servers such as Google keep a deleted object's.
		deleted, waiting, err := tx.TakeQueuedDelete(ctx, col.ID, o.Href)
		if err != nil {
			return err
		}
		if waiting {
			next.ETag = deleted.IfMatch
			break
		}
		name, err := p.newUID()
		if err != nil {
			return err
		}
		next.Href = col.Href + name + ".vcf"
	}
	id, err := tx.PutObject(ctx, next)
	if err != nil {
		return err
	}
	if err := indexCard(ctx, tx, id, card); err != nil {
		return err
	}
	whole, err := pimsync.ContactPayload{Whole: true}.Encode()
	if err != nil {
		return err
	}
	if op, waiting, err := tx.QueuedPut(ctx, id); err != nil {
		return err
	} else if waiting {
		return tx.SetPIMOpPayload(ctx, op.ID, whole)
	}
	_, err = tx.QueuePIMOp(ctx, store.PIMOp{AccountID: col.AccountID, CollectionID: col.ID, ObjectID: &id,
		Kind: "put", Href: next.Href, IfMatch: next.ETag, Payload: whole})
	return err
}

// writableContact is a contact's object and address book, when the user
// can change it.
func (p people) writableContact(ctx context.Context, id int64) (store.Object, store.Collection, error) {
	obj, err := p.DB.GetObject(ctx, id)
	if err == nil && obj.Kind != store.ObjectVCard {
		err = store.ErrNotFound
	}
	if err != nil {
		return store.Object{}, store.Collection{}, apiError(err, fmt.Sprintf("contact %d", id))
	}
	col, err := p.DB.GetCollection(ctx, obj.CollectionID)
	if err != nil {
		return store.Object{}, store.Collection{}, err
	}
	if err := p.writable(ctx, col); err != nil {
		return store.Object{}, store.Collection{}, err
	}
	return obj, col, nil
}

func (p people) writable(ctx context.Context, col store.Collection) error {
	account, err := p.DB.GetAccount(ctx, col.AccountID)
	if err != nil {
		return err
	}
	if col.Kind != api.CollectionKindAddressbook || col.ReadOnly || account.ReadOnly {
		return api.Conflict("address book %q is read-only", col.Name)
	}
	return nil
}

// otherContact is another contact of the person a contact belongs to,
// which stays theirs after a delete; 0 when there is none.
func (p people) otherContact(ctx context.Context, id int64) int64 {
	row, err := p.DB.PersonOfContact(ctx, id)
	if err != nil {
		return 0
	}
	for _, other := range row.ContactIDs {
		if other != id {
			return other
		}
	}
	return 0
}

// written kicks the account's pass and returns the write: the contact,
// their person (found by personOf, a contact of theirs; 0 for none left)
// and the restore.
func (p people) written(ctx context.Context, accountID, contactID, personOf int64, restoreID string) (*api.ContactWrite, error) {
	if p.PIM != nil {
		p.PIM.Kick(accountID)
	}
	out := &api.ContactWrite{ContactID: contactID, RestoreID: restoreID}
	if personOf == 0 {
		return out, nil
	}
	row, err := p.DB.PersonOfContact(ctx, personOf)
	if errors.Is(err, store.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if out.Person, err = p.fullPerson(ctx, row); err != nil {
		return nil, err
	}
	return out, nil
}

func indexCard(ctx context.Context, tx *store.Tx, id int64, card *vcardx.Card) error {
	if err := tx.IndexContact(ctx, id, pimsync.ContactIndexOf(card)); err != nil {
		return err
	}
	return tx.RelinkPeople(ctx)
}

// fieldsOf turns the API's fields into a patch, checking the email
// addresses; vcardx checks the birthday. A nil list keeps the contact's,
// and an empty one clears it.
func fieldsOf(f api.ContactFields) (vcardx.Fields, error) {
	out := vcardx.Fields{GivenName: f.GivenName, FamilyName: f.FamilyName, Nickname: f.Nickname,
		Organization: f.Organization, Title: f.Title, Birthday: f.Birthday, Note: f.Note}
	if f.Emails != nil {
		emails := labeledOf(f.Emails)
		for _, e := range emails {
			if !emailAddress.MatchString(e.Value) {
				return vcardx.Fields{}, api.InvalidParams("%q is not an email address", e.Value)
			}
		}
		out.Emails = &emails
	}
	if f.Phones != nil {
		phones := labeledOf(f.Phones)
		out.Phones = &phones
	}
	if f.URLs != nil {
		urls := labeledOf(f.URLs)
		out.URLs = &urls
	}
	if f.Addresses != nil {
		addresses := make([]vcardx.Address, 0, len(f.Addresses))
		for _, a := range f.Addresses {
			addresses = append(addresses, vcardx.Address{Label: a.Label, Street: a.Street, Locality: a.Locality,
				Region: a.Region, Postcode: a.Postcode, Country: a.Country})
		}
		out.Addresses = &addresses
	}
	return out, nil
}

func labeledOf(values []api.LabeledValue) []vcardx.Labeled {
	out := make([]vcardx.Labeled, 0, len(values))
	for _, v := range values {
		out = append(out, vcardx.Labeled{Label: strings.TrimSpace(v.Label), Value: strings.TrimSpace(v.Value)})
	}
	return out
}

// newRestoreID is a restore's opaque ID.
func newRestoreID() (string, error) {
	uid, err := newUUID()
	if err != nil {
		return "", err
	}
	return "r-" + uid, nil
}

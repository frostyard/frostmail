package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/frostyard/frostmail/api"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

func (w *workspace) contactPhoto(cx *core.Scope, id int64) core.Node {
	data := core.NewSignalFunc(cx, []byte(nil), func(_, _ []byte) bool { return false })
	perform(w, func(ctx context.Context, c *api.Client) (*api.Photo, error) {
		return c.People().Photo(ctx, &api.PeoplePhotoParams{ID: id})
	}, func(p *api.Photo, err error) {
		if err != nil || p == nil {
			return
		}
		raw, err := base64.StdEncoding.DecodeString(p.Data)
		if err == nil {
			cx.Post(func() { data.Set(raw) })
		}
	})
	return core.Image{SourceFunc: func() core.ImageSource { return core.ImageSource{Data: data.Get()} }, Label: "Contact photo", Style: core.Style{Width: core.Pt(72), Height: core.Pt(72), CornerRadius: ui.RadiusFull}}
}
func (w *workspace) setContactPhoto(id int64, photo *api.Photo, load func()) {
	w.change(func(ctx context.Context, c *api.Client) error {
		_, err := c.People().SetPhoto(ctx, &api.PeopleSetPhotoParams{ID: id, Photo: photo})
		return err
	}, func() { w.closePanel(); load() })
}
func (w *workspace) pickContactPhoto(id int64, load func()) {
	w.cx.PickFiles(core.PickFilesOptions{Title: "Contact photo"}, func(files []core.File, err error) {
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		if len(files) == 0 {
			return
		}
		file := files[0]
		w.change(func(ctx context.Context, c *api.Client) error {
			const maxSize = 256 << 10
			r, err := file.Open()
			if err != nil {
				return err
			}
			defer r.Close()
			data, err := io.ReadAll(io.LimitReader(r, maxSize+1))
			if err != nil {
				return err
			}
			mime := http.DetectContentType(data)
			if len(data) > maxSize || (mime != "image/png" && mime != "image/jpeg") {
				return errors.New("Choose a PNG or JPEG no larger than 256 KiB")
			}
			_, err = c.People().SetPhoto(ctx, &api.PeopleSetPhotoParams{ID: id, Photo: &api.Photo{ContentType: mime, Data: base64.StdEncoding.EncodeToString(data)}})
			return err
		}, func() { w.closePanel(); load() })
	})
}

// Each labeled value has separate fields so punctuation inside a phone number,
// email address or label survives an edit. A blank extra row adds another value.
func (w *workspace) fullContact(c api.Contact, load func()) {
	fields := []editField{{"Nickname", c.Nickname, false}, {"Job title", c.Title, false}, {"Birthday", c.Birthday, false}}
	groups := []struct {
		name   string
		values []api.LabeledValue
	}{{"Email", c.Emails}, {"Phone", c.Phones}, {"URL", c.URLs}}
	for _, g := range groups {
		for i := 0; i <= len(g.values); i++ {
			var v api.LabeledValue
			if i < len(g.values) {
				v = g.values[i]
			}
			label := fmt.Sprintf("%s %d", g.name, i+1)
			fields = append(fields, editField{label + " label", v.Label, false}, editField{label, v.Value, false})
		}
	}
	for i := 0; i <= len(c.Addresses); i++ {
		var a api.PostalAddress
		if i < len(c.Addresses) {
			a = c.Addresses[i]
		}
		prefix := fmt.Sprintf("Address %d ", i+1)
		for _, f := range []editField{{"label", a.Label, false}, {"street", a.Street, true}, {"city", a.Locality, false}, {"region", a.Region, false}, {"postal code", a.Postcode, false}, {"country", a.Country, false}} {
			f.label = prefix + f.label
			fields = append(fields, f)
		}
	}
	w.form("Contact details", fields, func(v []string) error {
		p := api.ContactFields{Nickname: changed(c.Nickname, v[0]), Title: changed(c.Title, v[1]), Birthday: changed(c.Birthday, v[2])}
		pos := 3
		for i, g := range groups {
			out := make([]api.LabeledValue, 0, len(g.values)+1)
			modified := false
			for j := 0; j <= len(g.values); j++ {
				value := api.LabeledValue{Label: v[pos], Value: v[pos+1]}
				pos += 2
				if strings.TrimSpace(value.Value) != "" {
					out = append(out, value)
				}
				if j < len(g.values) {
					modified = modified || value != g.values[j]
				} else {
					modified = modified || value.Value != ""
				}
			}
			if modified {
				switch i {
				case 0:
					p.Emails = out
				case 1:
					p.Phones = out
				case 2:
					p.URLs = out
				}
			}
		}
		addrs := make([]api.PostalAddress, 0, len(c.Addresses)+1)
		modified := false
		for i := 0; i <= len(c.Addresses); i++ {
			a := api.PostalAddress{Label: v[pos], Street: v[pos+1], Locality: v[pos+2], Region: v[pos+3], Postcode: v[pos+4], Country: v[pos+5]}
			pos += 6
			if strings.TrimSpace(a.Street+a.Locality+a.Region+a.Postcode+a.Country) != "" {
				addrs = append(addrs, a)
			}
			if i < len(c.Addresses) {
				modified = modified || a != c.Addresses[i]
			} else {
				modified = modified || len(addrs) > len(c.Addresses)
			}
		}
		if modified {
			p.Addresses = addrs
		}
		w.writeContact(func(ctx context.Context, client *api.Client) (*api.ContactWrite, error) {
			return client.People().Update(ctx, &api.PeopleUpdateParams{ID: c.ID, Fields: p})
		}, load)
		return nil
	})
}

// Keep the undo token with the daemon that issued it.
func (w *workspace) writeContact(op func(context.Context, *api.Client) (*api.ContactWrite, error), load func()) {
	if w.acting.GetUntracked() {
		return
	}
	w.acting.Set(true)
	target := w.session.Target()
	perform(w, op, func(result *api.ContactWrite, err error) {
		w.acting.Set(false)
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		w.closePanel()
		load()
		if result.RestoreID == "" {
			return
		}
		w.toaster.Show(ui.ToastContent{Title: "Address book updated", Action: "Undo contact change", OnAction: func() {
			if !w.session.Target().Same(target) {
				return
			}
			w.change(func(ctx context.Context, c *api.Client) error {
				return c.People().Restore(ctx, &api.PeopleRestoreParams{RestoreID: result.RestoreID})
			}, nil)
		}})
	})
}

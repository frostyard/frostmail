package store

import (
	"testing"

	"github.com/frostyard/frostmail/api"
)

func TestSMTPMaxSizeDefault(t *testing.T) {
	d, _ := openTest(t)
	in := sampleAccount("size@mailtest.test")
	in.SMTPMaxSize = 123 // an insert must not copy a previously learned limit
	a := insertAccount(t, d, in)
	if a.SMTPMaxSize != 0 {
		t.Errorf("InsertAccount SMTPMaxSize = %d, want 0", a.SMTPMaxSize)
	}
	got, err := d.GetAccount(t.Context(), a.ID)
	if err != nil || got.SMTPMaxSize != 0 {
		t.Fatalf("GetAccount SMTPMaxSize = %d, %v; want 0", got.SMTPMaxSize, err)
	}
}

func TestSetSMTPMaxSize(t *testing.T) {
	for _, tc := range []struct {
		name    string
		size    int64
		missing bool
		wantErr bool
	}{
		{"known", 10_000_000, false, false},
		{"none", 0, false, false},
		{"missing", 10_000_000, true, false},
		{"negative", -1, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, events := openTest(t)
			a := insertAccount(t, d, sampleAccount("size@mailtest.test"))
			id := a.ID
			if tc.missing {
				id = 9999
			}
			before := len(*events)
			err := d.Tx(t.Context(), func(tx *Tx) error { return tx.SetSMTPMaxSize(t.Context(), id, tc.size) })
			if (err != nil) != tc.wantErr {
				t.Fatalf("SetSMTPMaxSize(%d, %d) = %v, want error %v", id, tc.size, err, tc.wantErr)
			}
			want := tc.size
			if tc.missing || tc.wantErr {
				want = 0
			}
			got, err := d.GetAccount(t.Context(), a.ID)
			if err != nil || got.SMTPMaxSize != want {
				t.Errorf("GetAccount SMTPMaxSize = %d, %v; want %d", got.SMTPMaxSize, err, want)
			}
			if len(*events) != before {
				t.Error("SetSMTPMaxSize emitted an event")
			}
		})
	}
}

func TestUpdateAccountSMTPMaxSize(t *testing.T) {
	for _, name := range []string{"display name", "identical SMTP", "different host", "different port", "different TLS", "different username"} {
		t.Run(name, func(t *testing.T) {
			d, _ := openTest(t)
			a := insertAccount(t, d, sampleAccount("size@mailtest.test"))
			if err := d.Tx(t.Context(), func(tx *Tx) error { return tx.SetSMTPMaxSize(t.Context(), a.ID, 10_000_000) }); err != nil {
				t.Fatal(err)
			}
			smtp, displayName := a.SMTP, "Renamed"
			u := AccountUpdate{SMTP: &smtp}
			want := int64(0)
			switch name {
			case "display name":
				u = AccountUpdate{DisplayName: &displayName}
				want = 10_000_000
			case "identical SMTP":
				want = 10_000_000
			case "different host":
				smtp.Host = "other.mailtest.test"
			case "different port":
				smtp.Port = 465
			case "different TLS":
				smtp.TLS = api.TLSModeTLS
			case "different username":
				smtp.Username = "other"
			}
			if err := d.Tx(t.Context(), func(tx *Tx) error {
				got, err := tx.UpdateAccount(t.Context(), a.ID, u)
				if err == nil && got.SMTPMaxSize != want {
					t.Errorf("UpdateAccount SMTPMaxSize = %d, want %d", got.SMTPMaxSize, want)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			got, err := d.GetAccount(t.Context(), a.ID)
			if err != nil || got.SMTPMaxSize != want {
				t.Errorf("GetAccount SMTPMaxSize = %d, %v; want %d", got.SMTPMaxSize, err, want)
			}
		})
	}
}

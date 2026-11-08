package store

import "testing"

// TestUpdateSnippetKeepsFieldsNotGiven: a partial HTML update leaves name
// unchanged, and a partial name update leaves HTML unchanged.
func TestUpdateSnippetKeepsFieldsNotGiven(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	var created Snippet
	if err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		created, err = tx.CreateSnippet(ctx, Snippet{Name: "Greeting", HTML: "<p>Hello</p>"})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	html := "<p>Welcome</p>"
	var got Snippet
	if err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		got, err = tx.UpdateSnippet(ctx, created.ID, SnippetUpdate{HTML: &html})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got.Name != "Greeting" || got.HTML != html {
		t.Fatalf("HTML-only update = %+v", got)
	}

	name := "Welcome"
	if err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		got, err = tx.UpdateSnippet(ctx, created.ID, SnippetUpdate{Name: &name})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got.Name != name || got.HTML != html {
		t.Fatalf("name-only update = %+v", got)
	}
}

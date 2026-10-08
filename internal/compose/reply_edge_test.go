package compose

import "testing"

func TestReplyRecipientsDropsEmptyAddresses(t *testing.T) {
	src := Source{To: []Address{{Addr: ""}, {Addr: "bob@x.test"}}, Cc: []Address{{Name: "Nobody"}}}
	to, cc := ReplyRecipients(src, []string{"ann@x.test"}, true)
	if len(to) != 1 || to[0].Addr != "bob@x.test" {
		t.Errorf("to = %+v, want only bob (the source has no From)", to)
	}
	if cc != nil {
		t.Errorf("cc = %+v, want nil", cc)
	}
}

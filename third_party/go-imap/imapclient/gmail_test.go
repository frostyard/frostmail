package imapclient_test

// Tests for the frostmail patch: Gmail X-GM-EXT-1 support and skipping of
// unknown FETCH attributes. See FROSTMAIL-PATCHES.md.

import (
	"bufio"
	"net"
	"reflect"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// scripted runs a fake server: each step checks a client command (without
// its tag) and answers with lines where TAG is replaced by the command's tag.
type step struct {
	want  string
	reply []string
}

func scripted(t *testing.T, steps []step) *imapclient.Client {
	t.Helper()
	cliConn, srvConn := net.Pipe()
	go func() {
		defer srvConn.Close()
		w := bufio.NewWriter(srvConn)
		r := bufio.NewReader(srvConn)
		w.WriteString("* OK [CAPABILITY IMAP4rev1 X-GM-EXT-1] ready\r\n")
		w.Flush()
		for _, s := range steps {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			tag, cmd, _ := strings.Cut(strings.TrimRight(line, "\r\n"), " ")
			if cmd != s.want {
				t.Errorf("command = %q\nwant      %q", cmd, s.want)
			}
			for _, l := range s.reply {
				w.WriteString(strings.ReplaceAll(l, "TAG", tag) + "\r\n")
			}
			w.Flush()
		}
		r.ReadString('\n') // wait for the client to close
	}()
	c := imapclient.New(cliConn, nil)
	t.Cleanup(func() { c.Close() })
	return c
}

func TestGmailFetchParsing(t *testing.T) {
	c := scripted(t, []step{{
		want: "UID FETCH 1:* (UID X-GM-LABELS)",
		reply: []string{
			`* 1 FETCH (X-GM-THRID 1278455344230334865 X-GM-MSGID 1278455344230334866 UID 4 ` +
				`X-GM-LABELS (\Inbox \Sent Important "Muy Importante" &AMk-t&AOk-) X-FUTURE (a "b" {3}`,
			`xyz))`,
			`* 2 FETCH (UID 5 X-GM-LABELS ())`,
			`TAG OK done`,
		},
	}})
	msgs, err := c.Fetch(imap.UIDSet{imap.UIDRange{Start: 1}}, &imap.FetchOptions{GmailLabels: true}).Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages", len(msgs))
	}
	m := msgs[0]
	if m.UID != 4 || m.GmailMsgID != 1278455344230334866 || m.GmailThreadID != 1278455344230334865 {
		t.Errorf("ids = uid %d msgid %d thrid %d", m.UID, m.GmailMsgID, m.GmailThreadID)
	}
	wantLabels := []string{`\Inbox`, `\Sent`, "Important", "Muy Importante", "Été"}
	if !reflect.DeepEqual(m.GmailLabels, wantLabels) {
		t.Errorf("labels = %q, want %q", m.GmailLabels, wantLabels)
	}
	if msgs[1].GmailLabels == nil || len(msgs[1].GmailLabels) != 0 {
		t.Errorf("empty label list = %#v, want non-nil empty", msgs[1].GmailLabels)
	}
}

func TestGmailRawSearch(t *testing.T) {
	c := scripted(t, []step{{
		want:  `UID SEARCH X-GM-RAW "has:attachment in:unread"`,
		reply: []string{"* SEARCH 4 7", "TAG OK done"},
	}})
	data, err := c.UIDSearch(&imap.SearchCriteria{GmailRaw: "has:attachment in:unread"}, nil).Wait()
	if err != nil {
		t.Fatal(err)
	}
	if got := data.AllUIDs(); !reflect.DeepEqual(got, []imap.UID{4, 7}) {
		t.Fatalf("UIDs = %v", got)
	}
}

func TestStoreGmailLabels(t *testing.T) {
	c := scripted(t, []step{{
		want:  `UID STORE 4 +X-GM-LABELS (\Starred "Muy Importante")`,
		reply: []string{`* 1 FETCH (UID 4 X-GM-LABELS (\Starred "Muy Importante"))`, "TAG OK done"},
	}})
	msgs, err := c.StoreGmailLabels(imap.UIDSetNum(4), imap.StoreFlagsAdd, []string{`\Starred`, "Muy Importante"}, false).Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || !reflect.DeepEqual(msgs[0].GmailLabels, []string{`\Starred`, "Muy Importante"}) {
		t.Fatalf("store reply = %+v", msgs)
	}
}

func TestSearchCriteriaAndJoinsGmailRaw(t *testing.T) {
	a := imap.SearchCriteria{GmailRaw: "from:a"}
	a.And(&imap.SearchCriteria{GmailRaw: "is:starred"})
	if a.GmailRaw != "from:a is:starred" {
		t.Fatalf("GmailRaw = %q", a.GmailRaw)
	}
}

func TestPartialBinaryResponse(t *testing.T) {
	c := scripted(t, []step{{
		want:  "UID FETCH 4 (UID BINARY.PEEK[1]<0.2048>)",
		reply: []string{"* 4 FETCH (UID 4 BINARY[1]<0> {5}", "hello)", "TAG OK done"},
	}})
	section := &imap.FetchItemBinarySection{Part: []int{1}, Partial: &imap.SectionPartial{Size: 2048}, Peek: true}
	msgs, err := c.Fetch(imap.UIDSetNum(4), &imap.FetchOptions{UID: true, BinarySection: []*imap.FetchItemBinarySection{section}}).Collect()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(msgs[0].FindBinarySection(section)); got != "hello" {
		t.Fatalf("partial BINARY = %q", got)
	}
}

// A BODYSTRUCTURE the parser rejects costs that message its structure, not
// the connection: the message comes back with the structure as received,
// the next message and the next command parse as usual. Literals inside
// the structure and parts run together (as Gmail sends them) survive the
// raw read.
func TestUnparsedBodyStructure(t *testing.T) {
	good := `(("TEXT" "PLAIN" ("CHARSET" "UTF-8") NIL NIL "7BIT" 5 1 NIL NIL NIL)` +
		`("APPLICATION" "PDF" ("NAME" {7}`
	c := scripted(t, []step{
		{
			want: "UID FETCH 1:* (UID BODYSTRUCTURE)",
			reply: []string{
				`* 1 FETCH (UID 7 BODYSTRUCTURE (("TEXT" "PLAIN" NIL NIL NIL "7BIT" (12) 1 NIL NIL NIL)("TEXT" "HTML" NIL NIL NIL "7BIT" 9 1) "ALTERNATIVE"))`,
				`* 2 FETCH (UID 8 BODYSTRUCTURE ` + good,
				`a b.pdf) NIL NIL "BASE64" 30 NIL ("ATTACHMENT" NIL) NIL NIL) "MIXED" ("BOUNDARY" "x") NIL NIL NIL))`,
				`TAG OK done`,
			},
		},
		{want: "NOOP", reply: []string{`TAG OK still here`}},
	})
	msgs, err := c.Fetch(imap.UIDSet{imap.UIDRange{Start: 1}}, &imap.FetchOptions{
		UID: true, BodyStructure: &imap.FetchItemBodyStructure{Extended: true},
	}).Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages", len(msgs))
	}
	bad := msgs[0]
	if bad.UID != 7 || bad.BodyStructure != nil || bad.BodyStructureErr == nil ||
		!strings.HasPrefix(bad.BodyStructureUnparsed, `(("TEXT" "PLAIN" NIL NIL NIL "7BIT" (12) 1`) ||
		!strings.HasSuffix(bad.BodyStructureUnparsed, `"ALTERNATIVE")`) {
		t.Errorf("rejected structure: uid %d, structure %v, err %v, unparsed %q",
			bad.UID, bad.BodyStructure, bad.BodyStructureErr, bad.BodyStructureUnparsed)
	}
	mp, ok := msgs[1].BodyStructure.(*imap.BodyStructureMultiPart)
	if !ok || len(mp.Children) != 2 || msgs[1].BodyStructureErr != nil {
		t.Fatalf("second structure = %#v, %v", msgs[1].BodyStructure, msgs[1].BodyStructureErr)
	}
	if pdf := mp.Children[1].(*imap.BodyStructureSinglePart); pdf.Params["name"] != "a b.pdf" || pdf.Size != 30 {
		t.Errorf("attachment = %+v", pdf)
	}
	if err := c.Noop().Wait(); err != nil {
		t.Errorf("the connection did not survive: %v", err)
	}
}

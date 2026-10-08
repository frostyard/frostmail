package main

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Trimming makes a big account's first sync small enough to check in:
// each mailbox keeps the newest messages whose headers the session
// fetched, and the session is rewritten as if the mailbox held only
// those. SELECT's EXISTS, STATUS's MESSAGES and UID SEARCH's results
// shrink; each UID FETCH asks for the kept messages among its own, or is
// dropped when it asked for none; and FETCH responses are renumbered. A
// sync that resumed (some messages came from an earlier session) becomes
// a first sync of the messages this session fetched. Trimming is meant
// for first syncs: a mailbox that changes while selected, or a command
// that changes it, ends with an error.

// command is what trimming needs of a command line.
type command struct {
	tag     string
	name    string // upper case, "UID FETCH" for UID commands
	mailbox string // SELECT's, EXAMINE's or STATUS's
	set     string // a UID command's set
	rest    string // what follows the set
}

func parseCommand(line string) command {
	var c command
	c.tag, line, _ = strings.Cut(line, " ")
	name, args, _ := strings.Cut(line, " ")
	c.name = strings.ToUpper(name)
	if c.name == "UID" {
		sub, more, _ := strings.Cut(args, " ")
		c.name += " " + strings.ToUpper(sub)
		c.set, c.rest, _ = strings.Cut(more, " ")
	}
	switch c.name {
	case "SELECT", "EXAMINE", "STATUS":
		if nodes, err := parseSeq(args); err == nil && len(nodes) > 0 {
			c.mailbox = nodes[0].text
		}
	}
	return c
}

// fetched is one FETCH response: its message number and UID (0 when the
// response has none).
func fetched(text string) (seq, uid uint32, ok bool) {
	star, rest, _ := strings.Cut(text, " ")
	num, rest, _ := strings.Cut(rest, " ")
	word, data, _ := strings.Cut(rest, " ")
	n, err := strconv.ParseUint(num, 10, 32)
	if star != "*" || err != nil || !strings.EqualFold(word, "FETCH") {
		return 0, 0, false
	}
	nodes, err := parseSeq(data)
	if err != nil || len(nodes) == 0 || nodes[0].kind != list {
		return uint32(n), 0, true
	}
	items := nodes[0].items
	for i := 0; i+1 < len(items); i += 2 {
		if items[i].upper() == "UID" {
			u, _ := strconv.ParseUint(items[i+1].text, 10, 32)
			return uint32(n), uint32(u), true
		}
	}
	return uint32(n), 0, true
}

// keptUIDs finds, for every mailbox the session selected, the newest keep
// messages its header fetches (those asking for ENVELOPE) returned, in
// ascending order.
func keptUIDs(xs []*exchange, keep int) map[string][]uint32 {
	got := map[string][]uint32{}
	selected := ""
	for _, x := range xs {
		c := parseCommand(x.msgs[0].text)
		switch c.name {
		case "SELECT", "EXAMINE":
			selected = c.mailbox
			if _, ok := got[selected]; !ok {
				got[selected] = nil // selected, fetched nothing: kept empty
			}
		case "CLOSE", "UNSELECT":
			selected = ""
		case "UID FETCH":
			if selected == "" || !strings.Contains(strings.ToUpper(c.rest), "ENVELOPE") {
				continue
			}
			for _, m := range x.msgs[1:] {
				if _, uid, ok := fetched(m.text); ok && uid != 0 {
					got[selected] = append(got[selected], uid)
				}
			}
		}
	}
	kept := map[string][]uint32{}
	for mb, uids := range got {
		slices.Sort(uids)
		uids = slices.Compact(uids)
		kept[mb] = slices.Clone(uids[max(0, len(uids)-keep):])
	}
	return kept
}

var statusMessages = regexp.MustCompile(`(?i)\bMESSAGES \d+`)

func trim(xs []*exchange, keep int) ([]*exchange, error) {
	kept := keptUIDs(xs, keep)
	var out []*exchange
	selected := ""
	for _, x := range xs {
		c := parseCommand(x.msgs[0].text)
		uids, trimmed := kept[selected]
		switch c.name {
		case "SELECT", "EXAMINE":
			selected = c.mailbox
			n := len(kept[selected])
			x.edit(func(text string) (string, bool) {
				if num, word, ok := untagged(text); ok && word == "EXISTS" && num != "" {
					return fmt.Sprintf("* %d EXISTS", n), true
				}
				return text, true
			})
			out = append(out, x)
			continue
		case "CLOSE", "UNSELECT":
			selected = ""
		case "STATUS":
			if k, ok := kept[c.mailbox]; ok {
				x.edit(func(text string) (string, bool) {
					if _, word, _ := untagged(text); word == "STATUS" {
						text = statusMessages.ReplaceAllString(text, fmt.Sprintf("MESSAGES %d", len(k)))
					}
					return text, true
				})
			}
		case "UID SEARCH":
			if trimmed {
				if err := x.trimSearch(uids); err != nil {
					return nil, err
				}
			}
		case "UID FETCH":
			if trimmed {
				asked, err := parseSet(c.set)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", x.tag, err)
				}
				both := slices.DeleteFunc(asked, func(u uint32) bool { _, ok := slices.BinarySearch(uids, u); return !ok })
				if len(both) == 0 {
					continue // the client would not ask
				}
				words := strings.SplitN(x.msgs[0].text, " ", 4) // tag, UID, FETCH, the rest
				x.msgs[0].text = strings.Join(words[:3], " ") + " " + formatSet(both) + " " + c.rest
				if err := x.trimFetch(uids); err != nil {
					return nil, err
				}
				out = append(out, x)
				continue
			}
		case "NOOP", "IDLE", "CAPABILITY", "ID", "LIST", "LSUB", "XLIST", "NAMESPACE", "ENABLE", "LOGOUT", "CHECK":
		default:
			if trimmed {
				return nil, fmt.Errorf("%s: imaprec -keep cannot rewrite %s in a trimmed mailbox; cut before it", x.tag, c.name)
			}
		}
		if trimmed {
			for _, m := range x.msgs[1:] {
				if num, word, _ := untagged(m.text); num != "" || word == "VANISHED" {
					return nil, fmt.Errorf("%s: the mailbox changed while selected (%.40q); cut before it", x.tag, m.text)
				}
			}
		}
		out = append(out, x)
	}
	return out, nil
}

// untagged splits an untagged response into its number (for EXISTS,
// EXPUNGE, FETCH and the like) and its upper-case keyword.
func untagged(text string) (num, word string, ok bool) {
	star, rest, _ := strings.Cut(text, " ")
	if star != "*" {
		return "", "", false
	}
	first, rest, _ := strings.Cut(rest, " ")
	if _, err := strconv.ParseUint(first, 10, 32); err == nil {
		word, _, _ = strings.Cut(rest, " ")
		return first, strings.ToUpper(word), true
	}
	return "", strings.ToUpper(first), true
}

// edit rewrites or drops (keep false) each of the server's messages.
func (x *exchange) edit(f func(text string) (string, bool)) {
	msgs := x.msgs[:1]
	for _, m := range x.msgs[1:] {
		if m.sent {
			msgs = append(msgs, m)
			continue
		}
		text, keep := f(m.text)
		if keep {
			msgs = append(msgs, msg{text: text})
		}
	}
	x.msgs = msgs
}

// trimSearch rewrites a UID SEARCH's results to the kept UIDs.
func (x *exchange) trimSearch(uids []uint32) error {
	var err error
	x.edit(func(text string) (string, bool) {
		_, word, _ := untagged(text)
		switch word {
		case "SEARCH":
			parts := []string{"* SEARCH"}
			for _, u := range uids {
				parts = append(parts, strconv.FormatUint(uint64(u), 10))
			}
			return strings.Join(parts, " "), true
		case "ESEARCH":
			fields := strings.Fields(text)
			var keepFields []string
			for i := 0; i < len(fields); i++ {
				switch strings.ToUpper(fields[i]) {
				case "ALL":
					i++ // the set
				case "COUNT", "MIN", "MAX":
					err = fmt.Errorf("%s: imaprec -keep cannot rewrite ESEARCH %s", x.tag, fields[i])
				default:
					keepFields = append(keepFields, fields[i])
				}
			}
			if len(uids) > 0 {
				keepFields = append(keepFields, "ALL", formatSet(uids))
			}
			return strings.Join(keepFields, " "), true
		}
		return text, true
	})
	return err
}

// trimFetch drops the FETCH responses of messages not kept and numbers the
// rest as in a mailbox of only the kept messages.
func (x *exchange) trimFetch(uids []uint32) error {
	var err error
	x.edit(func(text string) (string, bool) {
		seq, uid, ok := fetched(text)
		if !ok {
			if num, word, _ := untagged(text); num != "" || word == "VANISHED" {
				err = fmt.Errorf("%s: the mailbox changed while selected (%.40q); cut before it", x.tag, text)
			}
			return text, true
		}
		i, found := slices.BinarySearch(uids, uid)
		if uid == 0 {
			err = fmt.Errorf("%s: a FETCH response without a UID (message %d)", x.tag, seq)
		}
		if !found {
			return "", false
		}
		_, rest, _ := strings.Cut(text[2:], " ")
		return fmt.Sprintf("* %d %s", i+1, rest), true
	})
	return err
}

// parseSet reads a UID set without "*".
func parseSet(set string) ([]uint32, error) {
	var out []uint32
	for part := range strings.SplitSeq(set, ",") {
		lo, hi, isRange := strings.Cut(part, ":")
		a, err := strconv.ParseUint(lo, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("bad UID set %q", set)
		}
		b := a
		if isRange {
			if b, err = strconv.ParseUint(hi, 10, 32); err != nil {
				return nil, fmt.Errorf("bad UID set %q", set)
			}
		}
		for u := min(a, b); u <= max(a, b); u++ {
			out = append(out, uint32(u))
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// formatSet writes sorted UIDs as go-imap does: ascending, runs as ranges.
func formatSet(uids []uint32) string {
	var parts []string
	for i := 0; i < len(uids); {
		j := i
		for j+1 < len(uids) && uids[j+1] == uids[j]+1 {
			j++
		}
		if i == j {
			parts = append(parts, strconv.FormatUint(uint64(uids[i]), 10))
		} else {
			parts = append(parts, fmt.Sprintf("%d:%d", uids[i], uids[j]))
		}
		i = j + 1
	}
	return strings.Join(parts, ",")
}

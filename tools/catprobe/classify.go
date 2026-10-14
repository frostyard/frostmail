package main

import (
	"regexp"
	"strings"
)

// Category is one of Mail.app's categories.
type Category int

// Mail.app's categories, in its order.
const (
	Primary Category = iota
	Transactions
	Updates
	Promotions
)

var categoryNames = [...]string{"Primary", "Transactions", "Updates", "Promotions"}

func (c Category) String() string { return categoryNames[c] }

// Mail is what the classifier reads of a stored message: nothing it would
// have to fetch.
type Mail struct {
	From        string // the From address, lowercased
	Subject     string
	ListID      bool // a List-Id header
	Unsubscribe bool // a List-Unsubscribe header
	// Known: the sender is a contact or someone the user wrote to, or the
	// user wrote in the conversation.
	Known bool
}

// transactional are subject words of receipts, orders, deliveries,
// bookings and bills.
var transactional = regexp.MustCompile(`(?i)\b(receipts?|orders?|invoices?|payments?|paid|shipped|shipping|shipment|` +
	`delivered|delivery|deliveries|confirmation|confirmed|booking|reservations?|itinerary|tickets?|e-ticket|` +
	`boarding pass|statements?|refunds?|purchases?|transactions?|bill)\b`)

// automated are the local parts of senders that are machines.
var automated = regexp.MustCompile(`^(no-?reply|do-?not-?reply|notifications?|notify|alerts?|updates?|mailer-daemon|` +
	`postmaster|bounces?|info|news|support|account|accounts|security)([+._-]|$)`)

// Classify sorts a message into a Mail.app category by its headers: mail
// from people the user knows is Primary; then receipts and the like by
// their subject; mailing lists and machines' notices are Updates; other
// mail with an unsubscribe link is Promotions; the rest is Primary.
func Classify(m Mail) Category {
	if m.Known {
		return Primary
	}
	bulk := m.ListID || m.Unsubscribe || automated.MatchString(localPart(m.From))
	switch {
	case bulk && transactional.MatchString(m.Subject):
		return Transactions
	case m.ListID:
		return Updates
	case m.Unsubscribe:
		return Promotions
	case bulk:
		return Updates
	}
	return Primary
}

func localPart(addr string) string {
	local, _, _ := strings.Cut(addr, "@")
	return local
}

// gmailTabs are Gmail's inbox categories, one per message.
var gmailTabs = []string{"primary", "social", "promotions", "updates", "forums"}

// gmailExtra are Gmail's other categories, beside the tab.
var gmailExtra = []string{"purchases", "reservations"}

// fromGmail maps a message's Gmail categories to Mail.app's: purchases and
// reservations are Transactions; social, updates and forums are Updates.
// ok is false when Gmail gave no tab.
func fromGmail(cats map[string]bool) (c Category, ok bool) {
	if cats["purchases"] || cats["reservations"] {
		return Transactions, true
	}
	switch {
	case cats["primary"]:
		return Primary, true
	case cats["promotions"]:
		return Promotions, true
	case cats["social"], cats["updates"], cats["forums"]:
		return Updates, true
	}
	return Primary, false
}

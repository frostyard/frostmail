//go:build linux && integration

package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

// The picture tours name a picture at each main view of the client on the
// showcase fixture. A headless run keeps no pixels; on a target
// (make nsgo-live) each name is a PNG, and comparison/linux holds the last
// set a person kept (plan 0016, rule 10).

func TestPicturesOfMail(t *testing.T) {
	a := fixtureWorkspace(t)
	const subject = "Your trip to Lisbon: itinerary"
	a.WaitFor("label:"+subject, 8*time.Second)
	a.Screenshot("mail-list")
	// The To-Do Bar is taken beside the list alone. With a message open,
	// the picture of the Xvfb window is blank from about a second after
	// the bar opens (seen 2026-11-24, before this tour's order changed).
	a.Press("label:To-Do Bar")
	a.WaitFor("label:New task title", 5*time.Second)
	a.Screenshot("mail-todo-bar")
	a.Press("label:To-Do Bar")
	a.WaitForGone("label:New task title", 5*time.Second)
	// The pointer over a row shows its quick actions.
	a.Hover("id:messages >> label:" + subject)
	a.Screenshot("mail-row-hover")
	a.HoverOut()
	a.Press("label:" + subject)
	a.WaitFor("label:Message body", 8*time.Second)
	a.Screenshot("mail-message")
	a.Press("label:More")
	a.WaitFor("label:Show headers", 5*time.Second)
	a.Screenshot("mail-message-actions")
	a.Key("Escape")
	a.WaitForGone("label:Show headers", 5*time.Second)
	a.Press("label:Delete")
	a.WaitFor("label:Cancel", 5*time.Second)
	a.Screenshot("dialog-confirm")
	a.Press("label:Cancel")
	// Several rows chosen, and a search with no match.
	a.PressWith("id:messages >> label:Dinner Saturday?", core.Modifiers{Control: true})
	a.WaitFor("text:2 messages selected", 5*time.Second)
	a.Screenshot("mail-several-selected")
	a.Type("label:Search All Inboxes", "no such words anywhere")
	a.Submit("label:Search All Inboxes")
	a.WaitFor("text:No matching messages", 8*time.Second)
	a.Screenshot("mail-no-match")
}

func TestPicturesOfAConversation(t *testing.T) {
	a := fixtureWorkspace(t)
	const subject = "Re: Aurora launch checklist"
	a.WaitFor("label:"+subject, 8*time.Second)
	a.Press("label:" + subject)
	a.WaitFor("label:View conversation (3 messages)", 5*time.Second)
	a.Press("label:View conversation (3 messages)")
	a.WaitFor("label:Open this message", 5*time.Second)
	a.Screenshot("conversation")
}

func TestPicturesOfCompose(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:Compose")
	a.WaitFor("label:Subject", 5*time.Second)
	a.Type("label:Subject", "Lunch on Thursday")
	a.Screenshot("compose")
}

func TestPicturesOfTheModules(t *testing.T) {
	a := fixtureWorkspace(t)
	for _, module := range []string{"Calendar", "People", "Tasks", "Settings", "Activity"} {
		a.Press("label:" + module)
		a.WaitUntil(5*time.Second, func() bool { return a.Exists("id:content-header >> text:" + module) })
		a.Settle()
		a.Screenshot("module-" + module)
	}
}

// The same views in Frostyard, the dark theme: the three panes with a
// message, a menu, a confirmation, compose, the Calendar and the narrow
// message.
func TestPicturesInDark(t *testing.T) {
	a := fixtureLook(t, "", 1280, 820, true)
	const subject = "Your trip to Lisbon: itinerary"
	a.WaitFor("label:"+subject, 8*time.Second)
	a.Press("label:" + subject)
	a.WaitFor("label:Message body", 8*time.Second)
	a.Screenshot("dark-mail-message")
	a.Press("label:More")
	a.WaitFor("label:Show headers", 5*time.Second)
	a.Screenshot("dark-mail-message-actions")
	a.Key("Escape")
	a.WaitForGone("label:Show headers", 5*time.Second)
	a.Press("label:Delete")
	a.WaitFor("label:Cancel", 5*time.Second)
	a.Screenshot("dark-dialog-confirm")
	a.Press("label:Cancel")
	a.Press("label:Calendar")
	a.WaitUntil(5*time.Second, func() bool { return a.Exists("id:content-header >> text:Calendar") })
	a.Settle()
	a.Screenshot("dark-module-Calendar")
	a.Press("label:Compose")
	a.WaitFor("label:Subject", 5*time.Second)
	a.Type("label:Subject", "Lunch on Thursday")
	a.Screenshot("dark-compose")
}

func TestPicturesOfANarrowWindowInDark(t *testing.T) {
	a := fixtureLook(t, "", 620, 820, true)
	const subject = "Your trip to Lisbon: itinerary"
	a.WaitFor("label:"+subject, 20*time.Second)
	a.Press("label:" + subject)
	a.WaitFor("label:Message body", 8*time.Second)
	a.Screenshot("dark-narrow-message")
}

func TestPicturesOfANarrowWindow(t *testing.T) {
	a := fixtureSized(t, "", 620, 820)
	const subject = "Your trip to Lisbon: itinerary"
	a.WaitFor("label:"+subject, 20*time.Second)
	a.Screenshot("narrow-list")
	a.Press("label:" + subject)
	a.WaitFor("label:Message body", 8*time.Second)
	a.Screenshot("narrow-message")
}

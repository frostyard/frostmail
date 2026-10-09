package main

import "strings"

// vocabulary protects syntax and values that steer parsing and replay.
var vocabulary = func() map[string]bool {
	words := `begin end vcard vcalendar vevent vtodo vjournal vfreebusy vtimezone standard daylight valarm
 version prodid calscale method publish request reply cancel refresh counter declinecounter
 fn n nickname org title role note email tel adr label url bday anniversary uid impp related categories photo logo
 summary description location comment contact organizer attendee related to resources attach
 dtstart dtend dtstamp created last modified due completed duration recurrence id rdate exdate rrule exrule
 freq until count interval bysecond byminute byhour byday bymonthday byyearday byweekno bymonth bysetpos wkst
 secondly minutely hourly daily weekly monthly yearly mo tu we th fr sa su
 tzid tzname tzoffsetfrom tzoffsetto tzurl sequence status class transp priority percent complete geo action trigger repeat
 type value encoding charset language pref altid pid mediatype calscale sort as cn cutype member role partstat rsvp
 delegated from sent by dir schedule agent force status range reltype fbtype fmttype related
 work home cell voice fax pager text video main other internet x400 uri date time date-time boolean integer float
 binary base64 png jpeg jpg gif data mailto http https urn uuid utc utf ascii xml json application
 accepted declined tentative needs action needsaction completed cancelled confirmed in process opaque transparent
 individual group resource room unknown chair req participant opt nonparticipant true false yes no
 public private confidential sibling parent child start finish display audio email procedure
 contacts calendars events dav caldav carddav ietf params ns xmlns ical apple calendarserver google
 propfind proppatch report get put post delete head options patch multistatus response propstat prop href
 displayname calendar addressbook description data multiget query home set current user principal resourcetype
 collection sync token level getetag getctag supported privilege read write bind unbind all component comp name
 depth content length transfer accept authorization proxy cookie redacted etag host connection keep alive
 found not ok charset encoding error status tasks task tasklist tasklists lists users items kind id notes links link
 selflink webviewlink updated position hidden deleted maxresults showcompleted showdeleted showhidden updatedmin
 com net edu gov io example vcf ics the from of and or`
	m := map[string]bool{}
	for _, w := range strings.Fields(words) {
		m[w] = true
	}
	return m
}()

var cardFields = fieldSet("FN N NICKNAME ORG TITLE ROLE NOTE EMAIL TEL ADR LABEL URL BDAY ANNIVERSARY UID IMPP RELATED CATEGORIES")
var calendarFields = fieldSet("SUMMARY DESCRIPTION LOCATION COMMENT CONTACT URL UID RELATED-TO RESOURCES CATEGORIES ATTACH ORGANIZER ATTENDEE")

func fieldSet(words string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(words) {
		m[w] = true
	}
	return m
}

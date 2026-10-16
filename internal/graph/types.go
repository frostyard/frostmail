package graph

// The parts of Graph's resources Frostmail reads. Graph's JSON is kept
// whole beside the derived source (objects.remote), so a field missing
// here is not lost.

// Calendar is a calendar the account can see.
type Calendar struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Color             string `json:"color"`
	HexColor          string `json:"hexColor"`
	CanEdit           bool   `json:"canEdit"`
	IsDefaultCalendar bool   `json:"isDefaultCalendar"`
	ChangeKey         string `json:"changeKey"`
}

// ContactFolder is a contact folder besides the default one.
type ContactFolder struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// DateTimeTimeZone is Graph's local time with a zone name; Frostmail asks
// for UTC.
type DateTimeTimeZone struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

// EmailAddress names a person.
type EmailAddress struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// Recipient is an organizer.
type Recipient struct {
	EmailAddress EmailAddress `json:"emailAddress"`
}

// Attendee is an invitee and their answer.
type Attendee struct {
	Type   string `json:"type"` // required, optional, resource
	Status struct {
		Response string `json:"response"`
	} `json:"status"`
	EmailAddress EmailAddress `json:"emailAddress"`
}

// Recurrence is a series' pattern and range.
type Recurrence struct {
	Pattern struct {
		Type           string   `json:"type"`
		Interval       int      `json:"interval"`
		Month          int      `json:"month"`
		DayOfMonth     int      `json:"dayOfMonth"`
		DaysOfWeek     []string `json:"daysOfWeek"`
		FirstDayOfWeek string   `json:"firstDayOfWeek"`
		Index          string   `json:"index"`
	} `json:"pattern"`
	Range struct {
		Type                string `json:"type"` // endDate, noEnd, numbered
		StartDate           string `json:"startDate"`
		EndDate             string `json:"endDate"`
		RecurrenceTimeZone  string `json:"recurrenceTimeZone"`
		NumberOfOccurrences int    `json:"numberOfOccurrences"`
	} `json:"range"`
}

// Event is an event, a series' master, an exception or an occurrence.
type Event struct {
	ID             string `json:"id"`
	ETag           string `json:"@odata.etag"`
	ChangeKey      string `json:"changeKey"`
	ICalUID        string `json:"iCalUId"`
	Type           string `json:"type"` // singleInstance, seriesMaster, exception, occurrence
	SeriesMasterID string `json:"seriesMasterId"`
	OccurrenceID   string `json:"occurrenceId"`
	// OriginalStart is an exception's start before it moved (UTC).
	OriginalStart string `json:"originalStart"`
	Subject       string `json:"subject"`
	Body          struct {
		ContentType string `json:"contentType"`
		Content     string `json:"content"`
	} `json:"body"`
	Start                      DateTimeTimeZone `json:"start"`
	End                        DateTimeTimeZone `json:"end"`
	OriginalStartTimeZone      string           `json:"originalStartTimeZone"`
	OriginalEndTimeZone        string           `json:"originalEndTimeZone"`
	IsAllDay                   bool             `json:"isAllDay"`
	IsCancelled                bool             `json:"isCancelled"`
	IsOrganizer                bool             `json:"isOrganizer"`
	IsReminderOn               bool             `json:"isReminderOn"`
	ReminderMinutesBeforeStart int              `json:"reminderMinutesBeforeStart"`
	ShowAs                     string           `json:"showAs"`
	Sensitivity                string           `json:"sensitivity"`
	Categories                 []string         `json:"categories"`
	Location                   struct {
		DisplayName string `json:"displayName"`
	} `json:"location"`
	Organizer      *Recipient `json:"organizer"`
	Attendees      []Attendee `json:"attendees"`
	ResponseStatus struct {
		Response string `json:"response"`
	} `json:"responseStatus"`
	Recurrence    *Recurrence `json:"recurrence"`
	OnlineMeeting *struct {
		JoinURL string `json:"joinUrl"`
	} `json:"onlineMeeting"`
	CreatedDateTime      string `json:"createdDateTime"`
	LastModifiedDateTime string `json:"lastModifiedDateTime"`
	// A master's cancelled occurrences, as occurrence IDs, and its
	// exceptions ($expand=exceptionOccurrences).
	CancelledOccurrences []string `json:"cancelledOccurrences"`
	ExceptionOccurrences []Event  `json:"exceptionOccurrences"`
	// Removed is set on a delta round's deletions, which carry only an ID.
	Removed *struct {
		Reason string `json:"reason"`
	} `json:"@removed"`
}

// PhysicalAddress is a contact's postal address.
type PhysicalAddress struct {
	Street          string `json:"street"`
	City            string `json:"city"`
	State           string `json:"state"`
	CountryOrRegion string `json:"countryOrRegion"`
	PostalCode      string `json:"postalCode"`
}

// Contact is an Outlook contact.
type Contact struct {
	ID               string           `json:"id"`
	ETag             string           `json:"@odata.etag"`
	ChangeKey        string           `json:"changeKey"`
	ParentFolderID   string           `json:"parentFolderId"`
	DisplayName      string           `json:"displayName"`
	GivenName        string           `json:"givenName"`
	MiddleName       string           `json:"middleName"`
	Surname          string           `json:"surname"`
	Title            string           `json:"title"`
	Generation       string           `json:"generation"`
	NickName         string           `json:"nickName"`
	CompanyName      string           `json:"companyName"`
	Department       string           `json:"department"`
	JobTitle         string           `json:"jobTitle"`
	EmailAddresses   []EmailAddress   `json:"emailAddresses"`
	BusinessPhones   []string         `json:"businessPhones"`
	HomePhones       []string         `json:"homePhones"`
	MobilePhone      string           `json:"mobilePhone"`
	HomeAddress      *PhysicalAddress `json:"homeAddress"`
	BusinessAddress  *PhysicalAddress `json:"businessAddress"`
	OtherAddress     *PhysicalAddress `json:"otherAddress"`
	IMAddresses      []string         `json:"imAddresses"`
	BusinessHomePage string           `json:"businessHomePage"`
	Birthday         string           `json:"birthday"`
	PersonalNotes    string           `json:"personalNotes"`
	Categories       []string         `json:"categories"`
	LastModified     string           `json:"lastModifiedDateTime"`
	Removed          *struct {
		Reason string `json:"reason"`
	} `json:"@removed"`
}

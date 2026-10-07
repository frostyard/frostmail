package store

// Flags are a message's flags as stored (docs/design/storage.md).
type Flags struct {
	Seen      bool
	Flagged   bool
	Answered  bool
	Forwarded bool // the $Forwarded keyword
	Draft     bool
	Deleted   bool // \Deleted: marked for expunge; hidden from views
	// Color is 0 when unflagged, else 1-7: Mail.app's flag color
	// ($MailFlagBit0-2 read as a 3-bit number) plus one, so red, which has
	// no bits set, is 1.
	Color int
	// Keywords are the other keywords, sorted, without duplicates.
	Keywords []string
}

// FlagsFromIMAP maps IMAP flags and keywords to Flags. Task T-0015
// implements it.
func FlagsFromIMAP(flags []string) Flags {
	return Flags{}
}

// IMAPFlags maps Flags back to the IMAP flags and keywords that represent
// them. Task T-0015 implements it.
func (f Flags) IMAPFlags() []string {
	return nil
}

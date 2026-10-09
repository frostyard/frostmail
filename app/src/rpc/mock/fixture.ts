// Deterministic fixture data for MockTransport: one account with Mail.app's
// usual mailboxes, conversations, flags, attachments, plain and HTML bodies.
import type {
  Account,
  Address,
  Attendee,
  CalendarEvent,
  Collection,
  Contact,
  MessageSummary,
  Part,
  Person,
} from "../gen/api";
import type { MockCalendarData, MockEvent } from "./calendar";
import type { MockData, MockMailbox, MockMessage } from "./mock";
import type { MockPeopleData } from "./people";

/** FixtureOptions size the fixture. */
export interface FixtureOptions {
  /** Messages in the Inbox (default 60). */
  inbox?: number;
  /** The clock the dates count back from (default: now). */
  now?: Date;
  /** PRNG seed (default 1). */
  seed?: number;
  /** Seed two fired reminders, which raise the reminder window (default
   *  false: tests that render the main window need not mock window.open). */
  reminders?: boolean;
}

/** Mailbox IDs of the fixture account. */
export const FIXTURE = {
  accountId: 1,
  inbox: 1,
  drafts: 2,
  sent: 3,
  junk: 4,
  trash: 5,
  archive: 6,
  projects: 7,
  frost: 8,
  receipts: 9,
} as const;

const ME: Address = { name: "Test One", address: "test1@mailtest.test" };

const FIRST = ["Ann", "Bob", "Carmen", "Dmitri", "Elif", "Farah", "Gus", "Hiro", "Ines", "Jonas", "Kofi", "Lena"];
const LAST = ["Smith", "Okafor", "Lindqvist", "Moreau", "Tanaka", "Garcia", "Novak", "Haddad", "Kim", "Rossi"];
const COMPANIES = ["northwind", "acme", "vertex", "frostyard"];
const WORDS = (
  "budget review quarterly plan launch notes draft invoice meeting agenda design feedback release " +
  "schedule travel itinerary receipt update roadmap offsite proposal contract summary weekly report " +
  "kickoff migration backup server outage follow-up question lunch tickets workshop"
).split(" ");

/** mockData builds the fixture. */
export function mockData(opts: FixtureOptions = {}): MockData {
  const rand = mulberry32(opts.seed ?? 1);
  const now = opts.now ?? new Date();
  const pick = <T>(xs: readonly T[]): T => xs[Math.floor(rand() * xs.length)] as T;
  const person = (): Address => {
    const first = pick(FIRST);
    const last = pick(LAST);
    return { name: `${first} ${last}`, address: `${first}.${last}@${pick(COMPANIES)}.test`.toLowerCase() };
  };
  const sentence = (n: number) => {
    const s = Array.from({ length: n }, () => pick(WORDS)).join(" ");
    return s.charAt(0).toUpperCase() + s.slice(1);
  };

  const accounts: Account[] = [
    {
      id: FIXTURE.accountId,
      kind: "imap",
      email: ME.address,
      displayName: ME.name,
      auth: "password",
      imap: { host: "imap.mailtest.test", port: 993, tls: "tls", username: ME.address },
      smtp: { host: "smtp.mailtest.test", port: 587, tls: "starttls", username: ME.address },
      createdAt: "2026-01-01T00:00:00Z",
      readOnly: false,
      notify: true,
      syncDays: 0,
      signedIn: true,
    },
  ];
  const mb = (id: number, path: string, role: MockMailbox["role"]): MockMailbox => ({
    id,
    accountId: FIXTURE.accountId,
    path,
    name: path.split("/").pop() ?? path,
    delimiter: "/",
    role,
    label: false,
  });
  const mailboxes = [
    mb(FIXTURE.inbox, "INBOX", "inbox"),
    mb(FIXTURE.drafts, "Drafts", "drafts"),
    mb(FIXTURE.sent, "Sent", "sent"),
    mb(FIXTURE.junk, "Junk", "junk"),
    mb(FIXTURE.trash, "Trash", "trash"),
    mb(FIXTURE.archive, "Archive", "archive"),
    mb(FIXTURE.projects, "Projects", "none"),
    mb(FIXTURE.frost, "Projects/Frost", "none"),
    mb(FIXTURE.receipts, "Receipts", "none"),
  ];

  const messages: MockMessage[] = [];
  let nextId = 1;
  let nextThread = 1;
  const threadSize = new Map<number, number>();
  const make = (o: {
    mailbox: number;
    minutesAgo: number;
    from?: Address;
    subject: string;
    thread?: number;
    text: string;
    html?: string;
    seen?: boolean;
    flagColor?: number;
    attachments?: Part[];
    remote?: number;
    trackers?: number;
  }): MockMessage => {
    const id = nextId++;
    const thread = o.thread ?? nextThread++;
    threadSize.set(thread, (threadSize.get(thread) ?? 0) + 1);
    const from = o.from ?? person();
    const summary: MessageSummary = {
      id,
      accountId: FIXTURE.accountId,
      mailboxIds: [o.mailbox],
      threadId: thread,
      subject: o.subject,
      from,
      date: new Date(now.getTime() - o.minutesAgo * 60_000).toISOString(),
      preview: o.text
        .split("\n")
        .filter((l) => l.trim() !== "" && !l.startsWith(">"))
        .join(" ")
        .slice(0, 200),
      flags: {
        seen: o.seen ?? true,
        flagged: (o.flagColor ?? 0) > 0,
        answered: false,
        forwarded: false,
        draft: false,
        flagColor: o.flagColor ?? 0,
      },
      hasAttachments: (o.attachments ?? []).length > 0,
      size: 2000 + o.text.length,
      threadCount: 1,
    };
    const m: MockMessage = {
      summary,
      to: [ME],
      cc: [],
      parts: o.attachments ?? [],
      text: o.text,
      html: o.html ?? "",
      remote: o.remote ?? 0,
      trackers: o.trackers ?? 0,
    };
    messages.push(m);
    return m;
  };

  // A conversation across INBOX and Sent, newest reply unread.
  const ann: Address = { name: "Ann Smith", address: "ann.smith@northwind.test" };
  const conv = nextThread++;
  make({
    mailbox: FIXTURE.inbox,
    minutesAgo: 60 * 26,
    from: ann,
    subject: "Offsite plan",
    thread: conv,
    text: "Hi,\n\nShall we hold the offsite in Lisbon this year?\n\nAnn\n-- \nAnn Smith\nNorthwind",
  });
  make({
    mailbox: FIXTURE.sent,
    minutesAgo: 60 * 25,
    from: ME,
    subject: "Re: Offsite plan",
    thread: conv,
    text: "Lisbon works for me.\n\nOn Tue, Ann Smith wrote:\n> Hi,\n>\n> Shall we hold the offsite in Lisbon this year?",
  });
  make({
    mailbox: FIXTURE.inbox,
    minutesAgo: 35,
    from: ann,
    subject: "Re: Offsite plan",
    thread: conv,
    seen: false,
    flagColor: 1,
    text: [
      "Great, booking it. Agenda: https://northwind.test/offsite",
      "",
      "> Lisbon works for me.",
      ">",
      "> On Tue, Ann Smith wrote:",
      ">> Hi,",
      ">>",
      ">> Shall we hold the offsite in Lisbon this year?",
      ">> We could also do Porto.",
      ">> Or stay home.",
    ].join("\n"),
  });
  // An HTML newsletter with remote images and a tracker.
  make({
    mailbox: FIXTURE.inbox,
    minutesAgo: 90,
    from: { name: "Vertex News", address: "news@vertex.test" },
    subject: "October product update",
    text: "October product update\n\nNew dashboards, faster sync and a fresh look.",
    html: '<div style="font-family:sans-serif;max-width:600px"><h1 style="color:#1d4ed8">October product update</h1><p>New dashboards, faster sync and a fresh look.</p><p><a href="https://vertex.test/blog">Read the blog</a></p></div>',
    remote: 3,
    trackers: 1,
    seen: false,
  });
  // A message with attachments.
  make({
    mailbox: FIXTURE.inbox,
    minutesAgo: 60 * 50,
    from: { name: "Kofi Okafor", address: "kofi.okafor@acme.test" },
    subject: "Signed contract and invoice",
    text: "Attached are the signed contract and the invoice for September.\n\nKofi",
    attachments: [
      {
        path: "2",
        contentType: "application/pdf",
        filename: "contract-signed.pdf",
        disposition: "attachment",
        contentId: "",
        size: 482_113,
      },
      {
        path: "3",
        contentType: "text/csv",
        filename: "invoice-september.csv",
        disposition: "attachment",
        contentId: "",
        size: 2_310,
      },
    ],
    flagColor: 3,
  });

  const fill = Math.max(
    0,
    (opts.inbox ?? 60) - messages.filter((m) => m.summary.mailboxIds[0] === FIXTURE.inbox).length,
  );
  for (let i = 0; i < fill; i++) {
    const subject = sentence(3 + Math.floor(rand() * 5));
    const unread = rand() < 0.3;
    const flag = rand() < 0.08 ? 1 + Math.floor(rand() * 7) : 0;
    make({
      mailbox: FIXTURE.inbox,
      minutesAgo: 120 + i * 47 + Math.floor(rand() * 40),
      subject,
      text: `${sentence(12)}.\n\n${sentence(18)}.\n\nThanks`,
      seen: !unread,
      flagColor: flag,
    });
  }
  const other: [number, number][] = [
    [FIXTURE.archive, 8],
    [FIXTURE.sent, 5],
    [FIXTURE.frost, 6],
    [FIXTURE.receipts, 4],
    [FIXTURE.junk, 2],
    [FIXTURE.trash, 3],
  ];
  for (const [mailbox, n] of other) {
    for (let i = 0; i < n; i++) {
      make({ mailbox, minutesAgo: 300 + i * 700, subject: sentence(4), text: `${sentence(15)}.`, seen: rand() < 0.8 });
    }
  }
  for (const m of messages) m.summary.threadCount = threadSize.get(m.summary.threadId) ?? 1;
  const pim = fixturePeople(now);
  pim.collections.push(...fixtureCalendars());
  return { accounts, mailboxes, messages, pim, calendar: fixtureCalendar(now, opts.reminders === true) };
}

/** Calendar IDs of the fixture account. */
export const CALENDARS = { work: 201, home: 202, holidays: 203 } as const;

function fixtureCalendars(): Collection[] {
  const calendar = (id: number, name: string, color: string, readOnly: boolean): Collection => ({
    id,
    accountId: FIXTURE.accountId,
    kind: "calendar",
    name,
    color,
    events: true,
    tasks: false,
    readOnly,
    enabled: true,
    isDefault: id === CALENDARS.work,
  });
  return [
    calendar(CALENDARS.work, "Work", "#3366cc", false),
    calendar(CALENDARS.home, "Home", "#34c759", false),
    calendar(CALENDARS.holidays, "Holidays", "#ff9500", true),
  ];
}

const who = (
  email: string,
  name: string,
  answer: Attendee["answer"],
  role: Attendee["role"] = "required",
): Attendee => ({
  email,
  name,
  role,
  answer,
  isUser: email === ME.address,
});

// fixtureCalendar is a fortnight of events around now's UTC day D: a daily
// standup, a weekly sync, a review today, an invitation, a cancelled
// appointment, a holiday and a three-day conference.
function fixtureCalendar(now: Date, withReminders: boolean): MockCalendarData {
  const day = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
  const at = (days: number, hh: number, mm: number) =>
    new Date(day.getTime() + days * 86_400_000 + (hh * 60 + mm) * 60_000).toISOString();
  const date = (days: number) => at(days, 0, 0).slice(0, 10);
  const me = who(ME.address, "", "accepted");
  const base = {
    recurrenceId: "",
    accountId: FIXTURE.accountId,
    location: "",
    description: "",
    allDay: false,
    startDate: "",
    endDate: "",
    timeZone: "",
    recurring: false,
    recurrence: "",
    status: "confirmed",
    transparent: false,
    attendees: [],
    alarms: [],
    readOnly: false,
  } satisfies Partial<CalendarEvent>;
  const event = (e: Partial<CalendarEvent> & Pick<CalendarEvent, "id" | "calendarId" | "summary" | "start" | "end">) =>
    ({ ...base, uid: `fixture-${e.id}`, ...e }) as CalendarEvent;
  const allDay = (days: number, length: number) => ({
    allDay: true,
    start: at(days, 0, 0),
    end: at(days + length, 0, 0),
    startDate: date(days),
    endDate: date(days + length),
  });
  const events: MockEvent[] = [
    {
      event: event({
        id: 301,
        calendarId: CALENDARS.work,
        summary: "Standup",
        location: "Room 4",
        start: at(-7, 9, 0),
        end: at(-7, 9, 15),
        recurring: true,
        recurrence: "RRULE:FREQ=DAILY;COUNT=30",
        organizer: who("bob.okafor@acme.test", "Bob Okafor", "accepted", "chair"),
        attendees: [
          me,
          who("carmen@vertex.test", "Carmen Lindqvist", "accepted"),
          who("hiro.haddad@acme.test", "Hiro Haddad", "needsaction"),
        ],
        answer: "accepted",
        alarms: [10],
      }),
      every: "day",
      count: 30,
    },
    {
      event: event({
        id: 302,
        calendarId: CALENDARS.work,
        summary: "Design review",
        location: "Studio B",
        description: "Walk through the calendar views.\nBring screenshots.",
        start: at(0, 14, 0),
        end: at(0, 15, 30),
        organizer: { ...who(ME.address, ME.name, "accepted", "chair") },
        attendees: [
          who("carmen@vertex.test", "Carmen Lindqvist", "accepted"),
          who("ines.kim@vertex.test", "Ines Kim", "tentative", "optional"),
        ],
        alarms: [15],
      }),
    },
    {
      event: event({
        id: 303,
        calendarId: CALENDARS.home,
        summary: "Lunch with Ann",
        location: "Cafe Nord",
        start: at(1, 12, 0),
        end: at(1, 13, 0),
        organizer: who("ann.smith@northwind.test", "Ann Smith", "accepted", "chair"),
        attendees: [who(ME.address, "", "needsaction")],
        answer: "needsaction",
      }),
    },
    {
      event: event({
        id: 304,
        calendarId: CALENDARS.holidays,
        summary: "Holiday",
        ...allDay(4, 1),
        transparent: true,
        readOnly: true,
      }),
    },
    {
      event: event({ id: 305, calendarId: CALENDARS.work, summary: "Conference", location: "Lisbon", ...allDay(6, 3) }),
    },
    {
      event: event({
        id: 306,
        calendarId: CALENDARS.home,
        summary: "Dentist",
        start: at(2, 16, 0),
        end: at(2, 17, 0),
        status: "cancelled",
      }),
    },
    {
      event: event({
        id: 307,
        calendarId: CALENDARS.work,
        summary: "Weekly sync",
        start: at(-14, 15, 0),
        end: at(-14, 16, 0),
        recurring: true,
        recurrence: "RRULE:FREQ=WEEKLY;COUNT=10",
        organizer: who("farah.garcia@northwind.test", "Farah Garcia", "accepted", "chair"),
        attendees: [me, who("bob.okafor@acme.test", "Bob Okafor", "accepted")],
        answer: "accepted",
      }),
      every: "week",
      count: 10,
    },
  ];
  // The standup's alarm fired this morning; the review's is the next.
  const reminders = [
    { id: "reminder-standup", eventId: 301, recurrenceId: `${date(0)}T09:00:00.000Z`, dueAt: at(0, 8, 50) },
    { id: "reminder-review", eventId: 302, recurrenceId: "", dueAt: at(0, 13, 45) },
  ];
  return { events, reminders: withReminders ? reminders : [], now: now.toISOString() };
}

/** Address book IDs of the fixture account. */
export const BOOKS = { contacts: 101, shared: 102 } as const;

// fixturePeople is the fixture account's address books: Contacts, and a
// read-only Shared book that also has Elif.
function fixturePeople(now: Date): MockPeopleData {
  const collections: Collection[] = [
    {
      id: BOOKS.contacts,
      accountId: FIXTURE.accountId,
      kind: "addressbook",
      name: "Contacts",
      color: "",
      events: false,
      tasks: false,
      readOnly: false,
      enabled: true,
      isDefault: true,
    },
    {
      id: BOOKS.shared,
      accountId: FIXTURE.accountId,
      kind: "addressbook",
      name: "Shared",
      color: "",
      events: false,
      tasks: false,
      readOnly: true,
      enabled: true,
      isDefault: false,
    },
  ];
  let next = 201;
  const contact = (over: Partial<Contact>): Contact => ({
    id: next++,
    collectionId: BOOKS.contacts,
    accountId: FIXTURE.accountId,
    displayName: `${over.givenName ?? ""} ${over.familyName ?? ""}`.trim(),
    givenName: "",
    familyName: "",
    nickname: "",
    organization: "",
    title: "",
    emails: [],
    phones: [],
    addresses: [],
    urls: [],
    birthday: "",
    note: "",
    readOnly: false,
    ...over,
  });
  const person = (...contacts: Contact[]): Person => {
    const first = contacts[0] as Contact;
    return {
      id: first.id,
      displayName: first.displayName,
      organization: contacts.find((c) => c.organization !== "")?.organization ?? "",
      hasPhoto: false,
      contacts,
    };
  };
  const simple = (given: string, family: string, org: string, email: string, title = "") =>
    person(
      contact({
        givenName: given,
        familyName: family,
        organization: org,
        title,
        emails: [{ label: "work", value: email }],
      }),
    );
  const people: Person[] = [
    person(
      contact({
        givenName: "Ann",
        familyName: "Smith",
        organization: "Northwind",
        title: "Head of Operations",
        emails: [
          { label: "work", value: "ann.smith@northwind.test" },
          { label: "home", value: "ann@smith-family.test" },
        ],
        phones: [
          { label: "mobile", value: "+1 555 0142" },
          { label: "work", value: "+1 555 0100" },
        ],
        addresses: [
          {
            label: "work",
            street: "1 Harbour Way",
            locality: "Portland",
            region: "OR",
            postcode: "97201",
            country: "USA",
          },
        ],
        urls: [{ label: "", value: "https://northwind.test/" }],
        birthday: "--04-12",
        note: "Runs the offsite.\nPrefers Lisbon.",
      }),
    ),
    simple("Bob", "Okafor", "Acme", "bob.okafor@acme.test", "Engineer"),
    simple("Carmen", "Lindqvist", "Vertex", "carmen@vertex.test", "Designer"),
    simple("Dmitri", "Moreau", "", "dmitri.moreau@mailtest.test"),
    person(
      contact({
        givenName: "Elif",
        familyName: "Tanaka",
        organization: "Frostyard",
        emails: [{ label: "work", value: "elif@frostyard.test" }],
      }),
      contact({
        collectionId: BOOKS.shared,
        displayName: "Elif T.",
        nickname: "Elif",
        emails: [{ label: "", value: "elif@frostyard.test" }],
        phones: [{ label: "mobile", value: "+1 555 0177" }],
        readOnly: true,
      }),
    ),
    simple("Farah", "Garcia", "Northwind", "farah.garcia@northwind.test", "Finance"),
    simple("Gus", "Novak", "", "gus@novak.test"),
    simple("Hiro", "Haddad", "Acme", "hiro.haddad@acme.test"),
    simple("Ines", "Kim", "Vertex", "ines.kim@vertex.test", "Product Manager"),
    simple("Jonas", "Rossi", "", "jonas.rossi@mailtest.test"),
    person(
      contact({
        displayName: "Vertex News",
        organization: "Vertex",
        emails: [{ label: "", value: "news@vertex.test" }],
      }),
    ),
    person(contact({ displayName: "42 Club", emails: [{ label: "", value: "hello@42club.test" }] })),
  ];
  const services: MockPeopleData["services"] = {
    [FIXTURE.accountId]: [
      {
        service: "contacts",
        available: true,
        enabled: true,
        url: "https://dav.mailtest.test/",
        signedIn: true,
        lastSyncAt: new Date(now.getTime() - 5 * 60_000).toISOString(),
      },
      { service: "calendar", available: true, enabled: false, url: "", signedIn: true },
      { service: "tasks", available: true, enabled: false, url: "", signedIn: true },
    ],
  };
  return { collections, people, services };
}

function mulberry32(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

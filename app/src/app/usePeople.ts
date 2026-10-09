// People data, with event-driven refreshes and stale-request cancellation.
import { useCallback, useEffect, useMemo, useState } from "react";

import { useClient } from "../data/session";
import { useMail, useUI } from "../data/stores";
import type { AddressBookSection } from "../features/people/PeopleSidebar";
import type { BookLabel } from "../features/people/PersonPane";
import type { Client, Collection, Event, MessageSummary, Person, PersonSummary } from "../rpc/gen/api";
import { composeTo, startDraft } from "./compose";

/** writeToPerson starts a message and contains compose-window failures. */
export function writeToPerson(client: Client, email = "") {
  void (email ? composeTo(client, email) : startDraft(client, "new")).catch((err: unknown) =>
    console.warn("compose", err),
  );
}

/** personEmail finds the first email in a person's contacts. */
export function personEmail(person: Person | null): string {
  return person?.contacts.flatMap((contact) => contact.emails)[0]?.value ?? "";
}

// Refresh on entry, parameter changes and debounced daemon events. Each load
// returns a cancellation function so older responses never replace newer data.
function useRefresh(client: Client, active: boolean, eventName: Event["event"], delay: number, load: () => () => void) {
  useEffect(() => {
    if (!active) return;
    let stop = load();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const off = client.transport.onEvent((event) => {
      if (event.event !== eventName) return;
      clearTimeout(timer);
      timer = setTimeout(() => {
        stop();
        stop = load();
      }, delay);
    });
    return () => {
      clearTimeout(timer);
      stop();
      off();
    };
  }, [client, active, eventName, delay, load]);
}

function usePeopleList(client: Client, active: boolean) {
  const { peopleBook, peopleSearch, selectPerson } = useUI();
  const [people, setPeople] = useState<PersonSummary[]>([]);
  const load = useCallback(() => {
    let stopped = false;
    const params = {
      ...(peopleBook === "all" ? {} : { collectionId: peopleBook }),
      ...(peopleSearch === "" ? {} : { query: peopleSearch }),
    };
    void client.people
      .list(params)
      .then((result) => {
        if (stopped) return;
        setPeople(result);
        const selected = useUI.getState().peopleSelected;
        if (selected !== null && !result.some((person) => person.id === selected)) selectPerson(null);
      })
      .catch((err: unknown) => console.warn("people list", err));
    return () => {
      stopped = true;
    };
  }, [client, peopleBook, peopleSearch, selectPerson]);
  useRefresh(client, active, "people.changed", 100, load);
  return people;
}

function usePerson(client: Client, active: boolean) {
  const selected = useUI((state) => state.peopleSelected);
  const [person, setPerson] = useState<Person | null>(null);
  const load = useCallback(() => {
    let stopped = false;
    // A refetch of the same person keeps showing them until it answers;
    // another person shows only once loaded (the hook's return checks the ID).
    if (selected === null) setPerson(null);
    else
      void client.people
        .get({ id: selected })
        .then((result) => {
          if (!stopped) setPerson(result);
        })
        .catch((err: unknown) => console.warn("people get", err));
    return () => {
      stopped = true;
    };
  }, [client, selected]);
  useRefresh(client, active, "people.changed", 100, load);
  return active && person?.id === selected ? person : null;
}

function usePersonExtras(client: Client, person: Person | null) {
  const [recent, setRecent] = useState<MessageSummary[]>([]);
  const [photo, setPhoto] = useState<string | undefined>();
  useEffect(() => {
    let stopped = false;
    setRecent([]);
    setPhoto(undefined);
    const email = personEmail(person);
    if (email)
      void client.people
        .card({ email })
        .then((card) => {
          if (!stopped) setRecent(card.recent);
        })
        .catch((err: unknown) => console.warn("people card", err));
    if (person?.hasPhoto)
      void client.people
        .photo({ id: person.id })
        .then((result) => {
          if (!stopped) setPhoto(`data:${result.contentType};base64,${result.data}`);
        })
        .catch((err: unknown) => console.warn("people photo", err));
    return () => {
      stopped = true;
    };
  }, [client, person]);
  return { recent, photo };
}

function useAddressBooks(client: Client, active: boolean) {
  const accounts = useMail((state) => state.accounts);
  const [collections, setCollections] = useState<Collection[]>([]);
  const load = useCallback(() => {
    let stopped = false;
    void client.account
      .collections({ kind: "addressbook" })
      .then((result) => {
        if (!stopped) setCollections(result);
      })
      .catch((err: unknown) => console.warn("address books", err));
    return () => {
      stopped = true;
    };
  }, [client]);
  useRefresh(client, active, "account.changed", 0, load);
  return useMemo(() => {
    const sections: AddressBookSection[] = accounts
      .map((account) => ({
        accountId: account.id,
        title: account.email,
        books: collections
          .filter((book) => book.accountId === account.id)
          .map((book) => ({
            id: book.id,
            name: book.name,
            readOnly: book.readOnly || account.readOnly,
          })),
      }))
      .filter((section) => section.books.length > 0);
    const books: Record<number, BookLabel> = {};
    for (const section of sections) {
      for (const book of section.books) books[book.id] = { name: book.name, account: section.title };
    }
    return { sections, books };
  }, [accounts, collections]);
}

/** usePeople loads the People module's list, selection and address books. */
export function usePeople() {
  const client = useClient();
  const active = useUI((state) => state.module === "people");
  const people = usePeopleList(client, active);
  const person = usePerson(client, active);
  const extras = usePersonExtras(client, person);
  const addressBooks = useAddressBooks(client, active);
  return { people, person, ...extras, ...addressBooks };
}

/** PeopleData is the connected People module's data. */
export type PeopleData = ReturnType<typeof usePeople>;

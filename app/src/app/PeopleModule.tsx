// People panes share Mail's column widths and splitters.
import { defaultRangeExtractor, type Range, useVirtualizer, type VirtualItem } from "@tanstack/react-virtual";
import { invoke, isTauri } from "@tauri-apps/api/core";
import { forwardRef, type Ref, type RefObject, useEffect, useImperativeHandle, useMemo, useRef } from "react";
import { useClient } from "../data/session";
import { type Pane, useUI } from "../data/stores";
import { UpcomingList } from "../features/calendar/UpcomingList";
import { PeopleSidebar } from "../features/people/PeopleSidebar";
import { PersonPane } from "../features/people/PersonPane";
import { IndexHeader, PersonRow } from "../features/people/PersonRow";
import { ModuleBar } from "../features/sidebar/ModuleBar";
import type { Command } from "../lib/keymap";
import { type PeopleRow, peopleRows } from "../lib/peopleRows";
import type { PersonSummary } from "../rpc/gen/api";
import { revealMessage } from "./openMessage";
import { Splitter } from "./Splitter";
import { openOccurrence, useCalendarColors, useCalendarFrame } from "./useCalendar";
import { type PeopleData, writeToPerson } from "./usePeople";

/** PeopleHandle lets MainWindow cycle focus through the People panes. */
export interface PeopleHandle {
  focus: (pane: Pane) => void;
}

/** peopleCommand moves the single selection through the people list. */
export function peopleCommand(command: Command, people: PersonSummary[]): boolean {
  const ui = useUI.getState();
  const current = people.findIndex((person) => person.id === ui.peopleSelected);
  let index: number;
  switch (command) {
    case "previous":
      index = current < 0 ? 0 : Math.max(0, current - 1);
      break;
    case "next":
      index = Math.min(people.length - 1, current + 1);
      break;
    case "first":
      index = 0;
      break;
    case "last":
      index = people.length - 1;
      break;
    default:
      return false;
  }
  const person = people[index];
  if (person) ui.selectPerson(person.id);
  return true;
}

// Keep the active letter mounted even after it leaves the visible range.
function peopleRange(rows: PeopleRow[], range: Range): number[] {
  let header = range.startIndex;
  while (header > 0 && rows[header]?.kind !== "index") header--;
  return [...new Set([header, ...defaultRangeExtractor(range)])].sort((a, b) => a - b);
}

function stickyHeaderTop(rows: PeopleRow[], index: number, start: number, offset: number): number {
  let next = start + 24;
  for (let i = index + 1; i < rows.length; i++) {
    if (rows[i]?.kind === "index") return Math.min(Math.max(start, offset), next - 24);
    next += 44;
  }
  return Math.max(start, offset);
}

const observePeopleRect: NonNullable<
  Parameters<typeof useVirtualizer<HTMLDivElement, HTMLDivElement>>[0]["observeElementRect"]
> = (instance, callback) => {
  const element = instance.scrollElement;
  const measure = () => callback({ width: element?.clientWidth ?? 0, height: element?.clientHeight || 800 });
  measure();
  if (!element) return;
  const observer = new ResizeObserver(measure);
  observer.observe(element);
  return () => observer.disconnect();
};

function usePeopleVirtualizer(rows: PeopleRow[], scroller: RefObject<HTMLDivElement | null>, selected: number | null) {
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scroller.current,
    estimateSize: (index) => (rows[index]?.kind === "index" ? 24 : 44),
    getItemKey: (index) => {
      const row = rows[index];
      return row?.kind === "person" ? `person:${row.person.id}` : `index:${row?.letter}`;
    },
    observeElementRect: observePeopleRect,
    rangeExtractor: (range) => peopleRange(rows, range),
    overscan: 8,
  });
  useEffect(() => {
    const index = rows.findIndex((row) => row.kind === "person" && row.person.id === selected);
    if (index >= 0) virtualizer.scrollToIndex(index, { align: "auto" });
  }, [rows, selected, virtualizer]);
  return virtualizer;
}

interface VirtualRowProps {
  row: PeopleRow | undefined;
  item: VirtualItem;
  rows: PeopleRow[];
  offset: number;
  selected: number | null;
  focused: boolean;
  onSelect: (id: number) => void;
}

function PeopleVirtualRow({ row, item, rows, offset, selected, focused, onSelect }: VirtualRowProps) {
  const header = row?.kind === "index";
  const top = header ? stickyHeaderTop(rows, item.index, item.start, offset) : item.start;
  return (
    <div style={{ position: "absolute", top, left: 0, right: 0, zIndex: header ? 1 : 0 }}>
      {row?.kind === "index" ? (
        <IndexHeader letter={row.letter} />
      ) : (
        row && (
          <PersonRow person={row.person} selected={selected === row.person.id} focused={focused} onSelect={onSelect} />
        )
      )}
    </div>
  );
}

function PeopleList({ people }: { people: PersonSummary[] }) {
  const ui = useUI();
  const scroller = useRef<HTMLDivElement>(null);
  const rows = useMemo(() => peopleRows(people), [people]);
  const virtualizer = usePeopleVirtualizer(rows, scroller, ui.peopleSelected);
  const onSelect = (id: number) => {
    ui.selectPerson(id);
    ui.setPeopleFocus("list");
    scroller.current?.focus();
  };
  return (
    <div
      ref={scroller}
      role="listbox"
      aria-label="Contacts"
      tabIndex={0}
      onFocus={() => ui.setPeopleFocus("list")}
      className="h-full overflow-y-auto bg-window outline-none"
    >
      {people.length === 0 ? (
        <div className="flex h-full items-center justify-center text-empty text-secondary">
          {ui.peopleSearch ? "No Results" : "No Contacts"}
        </div>
      ) : (
        <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
          {virtualizer.getVirtualItems().map((item) => (
            <PeopleVirtualRow
              key={item.key}
              row={rows[item.index]}
              item={item}
              rows={rows}
              offset={virtualizer.scrollOffset ?? 0}
              selected={ui.peopleSelected}
              focused={ui.peopleFocus === "list"}
              onSelect={onSelect}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function PeopleDetail({ data }: { data: PeopleData }) {
  const frame = useCalendarFrame();
  const colors = useCalendarColors();
  const client = useClient();
  const openMessage = (id: number) => {
    void revealMessage(client, id)
      .then(() => useUI.getState().setModule("mail"))
      .catch((err: unknown) => console.warn("open message", err));
  };
  const openURL = (url: string) => {
    if (isTauri()) void invoke("open_link", { url }).catch((err: unknown) => console.warn("open link", err));
  };
  return (
    <PersonPane
      person={data.person}
      books={data.books}
      recent={data.recent}
      photo={data.photo}
      now={frame.now}
      upcoming={
        <UpcomingList
          occurrences={data.upcoming}
          colors={colors}
          timeZone={frame.timeZone}
          locale={frame.locale}
          now={frame.now}
          onOpen={(occurrence) => openOccurrence(occurrence, frame.timeZone)}
        />
      }
      onCompose={(email) => writeToPerson(client, email)}
      onOpenMessage={openMessage}
      onOpenURL={openURL}
    />
  );
}

function PeopleSidebarPane({ data, elementRef }: { data: PeopleData; elementRef: Ref<HTMLFieldSetElement> }) {
  const ui = useUI();
  return (
    <fieldset
      ref={elementRef}
      aria-label="People sidebar"
      onFocus={() => ui.setPeopleFocus("sidebar")}
      className="m-0 flex h-full min-w-0 shrink-0 flex-col border-0 bg-sidebar p-0"
      style={{ width: ui.sidebarWidth }}
    >
      <div className="min-h-0 flex-1 overflow-y-auto">
        <PeopleSidebar
          sections={data.sections}
          selected={ui.peopleBook}
          focused={ui.peopleFocus === "sidebar"}
          onSelect={(book) => {
            ui.setPeopleBook(book);
            ui.setPeopleFocus("sidebar");
          }}
        />
      </div>
      <ModuleBar modules={["mail", "calendar", "people", "tasks"]} current={ui.module} onSelect={ui.setModule} />
    </fieldset>
  );
}

interface PeopleModuleProps {
  data: PeopleData;
  resizeSidebar: (delta: number) => void;
  resizeList: (delta: number) => void;
  endDrag: () => void;
}

/** PeopleModule shows address books, a virtualized list and the selected person. */
export const PeopleModule = forwardRef<PeopleHandle, PeopleModuleProps>(function PeopleModule(props, ref) {
  const ui = useUI();
  const sidebar = useRef<HTMLFieldSetElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const reader = useRef<HTMLElement>(null);
  useImperativeHandle(
    ref,
    () => ({
      focus: (pane) => {
        if (pane === "sidebar") sidebar.current?.querySelector<HTMLElement>("[tabindex='0']")?.focus();
        else if (pane === "list") list.current?.querySelector<HTMLElement>("[role='listbox']")?.focus();
        else reader.current?.focus();
      },
    }),
    [],
  );
  return (
    <div className="flex min-h-0 flex-1">
      {ui.sidebarVisible && (
        <>
          <PeopleSidebarPane elementRef={sidebar} data={props.data} />
          <Splitter label="Resize sidebar" onResize={props.resizeSidebar} onEnd={props.endDrag} />
        </>
      )}
      <div ref={list} className="h-full shrink-0" style={{ width: ui.listWidth }}>
        <PeopleList people={props.data.people} />
      </div>
      <Splitter label="Resize contacts list" onResize={props.resizeList} onEnd={props.endDrag} />
      <section
        ref={reader}
        aria-label="Contact details"
        tabIndex={-1}
        onFocus={() => ui.setPeopleFocus("reader")}
        className="h-full min-w-0 flex-1 outline-none"
      >
        <PeopleDetail data={props.data} />
      </section>
    </div>
  );
});

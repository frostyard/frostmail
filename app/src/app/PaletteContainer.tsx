// The command palette's container: it gives the palette the module's
// commands and runs the one chosen through the window's own dispatch
// (docs/specs/ui.md, Command palette). Focus returns to where it was
// before the command runs, so a command that moves it keeps it there.

import { useEffect, useRef } from "react";

import { useUI } from "../data/stores";
import { CommandPalette } from "../features/palette/CommandPalette";
import { type Command, paletteItems } from "../lib/keymap";
import { IN_LODGE } from "../lodge/lodge";

/** PaletteContainerProps takes the main window's dispatch, the palette's run. */
export interface PaletteContainerProps {
  run: (command: Command) => void;
}

/** PaletteContainer draws the palette while it is open. */
export function PaletteContainer(props: PaletteContainerProps) {
  const open = useUI((state) => state.palette);
  const module = useUI((state) => state.module);
  const messages = useUI((state) => state.selected.length);
  const occurrence = useUI((state) => state.calendarSelected !== null);
  const task = useUI((state) => state.tasksSelected !== null);
  const person = useUI((state) => state.peopleSelected !== null);
  // A contact being edited has the window's keys, so it has no palette.
  const editing = useUI((state) => state.peopleEditing !== null);
  // The chosen command waits for the sheet to go, which restores focus first.
  const pending = useRef<Command | null>(null);
  const { run } = props;
  useEffect(() => {
    if (open) return;
    const command = pending.current;
    if (command === null) return;
    pending.current = null;
    run(command);
  }, [open, run]);
  if (!open || (module === "people" && editing)) return null;
  const count =
    module === "mail"
      ? messages
      : module === "calendar"
        ? Number(occurrence)
        : module === "tasks"
          ? Number(task)
          : Number(person);
  return (
    <CommandPalette
      items={paletteItems(module, count, { web: IN_LODGE })}
      onRun={(command) => {
        pending.current = command;
        useUI.getState().openPalette(false);
      }}
      onClose={() => useUI.getState().openPalette(false)}
    />
  );
}

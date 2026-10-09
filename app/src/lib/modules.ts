// The main window's modules (docs/specs/pim-ui.md, Modules; ADR-0020).

/** Module is one of the main window's four modules. */
export type Module = "mail" | "calendar" | "people" | "tasks";

/** MODULES lists the modules in their order: the module bar's and Ctrl+1–4's. */
export const MODULES: readonly Module[] = ["mail", "calendar", "people", "tasks"];

/** MODULE_LABEL names each module. */
export const MODULE_LABEL: Record<Module, string> = {
  mail: "Mail",
  calendar: "Calendar",
  people: "People",
  tasks: "Tasks",
};

/** MODULE_SHORTCUT is the key that shows each module. */
export const MODULE_SHORTCUT: Record<Module, string> = {
  mail: "Ctrl+1",
  calendar: "Ctrl+2",
  people: "Ctrl+3",
  tasks: "Ctrl+4",
};

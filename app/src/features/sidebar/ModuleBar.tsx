// The module bar at the bottom of every sidebar (docs/specs/pim-ui.md,
// Module bar). Task T-0063 writes it.
import type { Module } from "../../lib/modules";

/** ModuleBarProps are the module bar's inputs. */
export interface ModuleBarProps {
  /** The modules that are built, in module order. */
  modules: readonly Module[];
  current: Module;
  onSelect: (m: Module) => void;
}

/** ModuleBar switches between the window's modules. */
export function ModuleBar(_props: ModuleBarProps) {
  return null;
}

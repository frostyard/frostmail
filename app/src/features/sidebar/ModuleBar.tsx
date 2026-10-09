// The module bar at the bottom of every sidebar (docs/specs/pim-ui.md,
// Module bar), with a button and shortcut tooltip for each built module.
import { Calendar, ListChecks, type LucideIcon, Mail, Users } from "lucide-react";

import { MODULE_LABEL, MODULE_SHORTCUT, type Module } from "../../lib/modules";

const ICONS: Record<Module, LucideIcon> = { mail: Mail, calendar: Calendar, people: Users, tasks: ListChecks };

/** ModuleBarProps are the module bar's inputs. */
export interface ModuleBarProps {
  /** The modules that are built, in module order. */
  modules: readonly Module[];
  current: Module;
  onSelect: (m: Module) => void;
}

/** ModuleBar switches between the window's modules. */
export function ModuleBar({ modules, current, onSelect }: ModuleBarProps) {
  return (
    <div
      role="toolbar"
      aria-label="Modules"
      className="flex h-[44px] shrink-0 items-center gap-1 border-t border-separator bg-sidebar px-3"
    >
      {modules.map((module) => {
        const Icon = ICONS[module];
        return (
          <button
            key={module}
            type="button"
            aria-label={MODULE_LABEL[module]}
            title={`${MODULE_LABEL[module]} (${MODULE_SHORTCUT[module]})`}
            aria-pressed={module === current}
            onClick={() => onSelect(module)}
            className="flex h-7 w-9 items-center justify-center rounded-md hover:bg-selection-inactive"
          >
            <Icon size={18} aria-hidden="true" className={module === current ? "text-accent" : "text-secondary"} />
          </button>
        );
      })}
    </div>
  );
}

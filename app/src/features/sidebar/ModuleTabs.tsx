// A narrow window's modules (docs/specs/ui.md, Narrow windows): a tab bar
// at the window's foot, in place of the sidebars' module bar, so Mail,
// Calendar, People and Tasks are a tap from each other on a phone.
import { MODULE_LABEL, type Module } from "../../lib/modules";
import { MODULE_ICONS, type ModuleBarProps } from "./ModuleBar";

/** ModuleTabs switches between the window's modules in a narrow window. */
export function ModuleTabs({ modules, current, onSelect }: ModuleBarProps) {
  return (
    <div
      role="toolbar"
      aria-label="Modules"
      className="flex shrink-0 items-stretch gap-1 border-t border-separator bg-toolbar px-1 pt-1 pb-[max(4px,env(safe-area-inset-bottom))]"
    >
      {modules.map((module: Module) => {
        const Icon = MODULE_ICONS[module];
        const tone = module === current ? "text-accent" : "text-secondary";
        return (
          <button
            key={module}
            type="button"
            aria-label={MODULE_LABEL[module]}
            aria-pressed={module === current}
            onClick={() => onSelect(module)}
            className="flex h-11 min-w-0 flex-1 flex-col items-center justify-center gap-0.5 rounded-md hover:bg-selection-inactive"
          >
            <Icon size={22} aria-hidden="true" className={tone} />
            <span className={`text-[10px] leading-3 ${tone}`}>{MODULE_LABEL[module]}</span>
          </button>
        );
      })}
    </div>
  );
}

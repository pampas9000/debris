import { For } from "solid-js";
import { cn } from "../../lib/utils";

export interface SegmentedOption<T extends string> {
  value: T;
  label: string;
}

export interface SegmentedControlProps<T extends string> {
  value: T;
  options: readonly SegmentedOption<T>[];
  onChange: (value: T) => void;
  disabled?: boolean;
  size?: "sm" | "default";
  fullWidth?: boolean;
  ariaLabel?: string;
  class?: string;
}

export function SegmentedControl<T extends string>(props: SegmentedControlProps<T>) {
  return (
    <div
      role="radiogroup"
      aria-label={props.ariaLabel}
      class={cn(
        "inline-grid auto-cols-fr grid-flow-col gap-1 rounded-md bg-secondary p-0.5",
        props.fullWidth && "grid w-full",
        props.class
      )}
    >
      <For each={props.options}>
        {(opt) => {
          const active = () => props.value === opt.value;
          return (
            <button
              type="button"
              role="radio"
              aria-checked={active() ? "true" : "false"}
              disabled={props.disabled}
              onClick={() => props.onChange(opt.value)}
              class={cn(
                "rounded-sm font-medium transition-all cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50",
                props.size === "sm" ? "px-2 py-1 text-[11px]" : "px-2.5 py-1 text-xs",
                active()
                  ? "bg-card text-foreground shadow-xs"
                  : "text-muted-foreground hover:text-foreground"
              )}
            >
              {opt.label}
            </button>
          );
        }}
      </For>
    </div>
  );
}

import { cn } from "../../lib/utils";

export interface SwitchProps {
  checked: boolean;
  disabled?: boolean;
  onChange: (checked: boolean) => void;
  label: string;
}

export function Switch(props: SwitchProps) {
  return (
    <label
      class={cn(
        "flex items-center justify-between rounded-md border border-border bg-card px-3 py-2 text-xs transition-colors",
        props.disabled ? "opacity-50 cursor-not-allowed" : "cursor-pointer hover:bg-accent/60"
      )}
    >
      <span class="text-foreground">{props.label}</span>
      <button
        type="button"
        role="switch"
        aria-checked={props.checked ? "true" : "false"}
        disabled={props.disabled}
        onClick={() => props.onChange(!props.checked)}
        class={cn(
          "relative inline-flex h-4.5 w-8 shrink-0 items-center rounded-full border border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background cursor-pointer",
          props.checked ? "bg-primary" : "bg-muted-foreground/30"
        )}
      >
        <span
          class={cn(
            "pointer-events-none block h-3.5 w-3.5 rounded-full bg-white shadow-xs transition-transform",
            props.checked ? "translate-x-3.5" : "translate-x-0.5"
          )}
        />
      </button>
    </label>
  );
}

import { Check } from "lucide";
import { Show } from "solid-js";
import { cn } from "../../lib/utils";
import { Icon } from "./icon";

export interface CheckboxProps {
  checked: boolean;
  disabled?: boolean;
  onChange: (checked: boolean) => void;
  label: string;
  class?: string;
}

export function Checkbox(props: CheckboxProps) {
  return (
    <div class={cn("inline-flex items-center gap-2", props.class)}>
      <button
        type="button"
        role="checkbox"
        aria-checked={props.checked ? "true" : "false"}
        disabled={props.disabled}
        onClick={() => props.onChange(!props.checked)}
        class={cn(
          "inline-flex h-4 w-4 shrink-0 items-center justify-center rounded-[4px] border transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background cursor-pointer disabled:cursor-not-allowed disabled:opacity-50",
          props.checked
            ? "border-primary bg-primary text-primary-foreground shadow-xs"
            : "border-input bg-card hover:border-primary/60"
        )}
      >
        <Show when={props.checked}>
          <Icon icon={Check} size={11} strokeWidth={3} />
        </Show>
      </button>
      <span
        onClick={() => {
          if (!props.disabled) props.onChange(!props.checked);
        }}
        class={cn(
          "text-xs text-foreground",
          props.disabled ? "opacity-50 cursor-not-allowed" : "cursor-pointer"
        )}
      >
        {props.label}
      </span>
    </div>
  );
}

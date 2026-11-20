import { Check, ChevronDown, type IconNode } from "lucide";
import { For, Show, createSignal, onSettled } from "solid-js";
import { cn } from "../../lib/utils";
import { Icon } from "./icon";

export interface SelectOption<T extends string> {
  value: T;
  label: string;
}

export interface SelectProps<T extends string> {
  value: T;
  options: readonly SelectOption<T>[];
  onChange: (value: T) => void;
  icon?: IconNode;
  ariaLabel?: string;
  class?: string;
  align?: "left" | "right";
}

export function Select<T extends string>(props: SelectProps<T>) {
  const [open, setOpen] = createSignal(false);
  let containerRef: HTMLDivElement | undefined;

  const currentLabel = () => props.options.find((o) => o.value === props.value)?.label ?? props.value;

  onSettled(() => {
    const onPointerDown = (e: PointerEvent) => {
      if (open() && containerRef && !containerRef.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (open() && e.key === "Escape") {
        setOpen(false);
      }
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  });

  return (
    <div ref={containerRef} class={cn("relative inline-block", props.class)}>
      <button
        type="button"
        aria-haspopup="listbox"
        aria-expanded={open() ? "true" : "false"}
        aria-label={props.ariaLabel}
        onClick={() => setOpen((v) => !v)}
        class="inline-flex h-7 items-center gap-1.5 rounded-md border border-input bg-card px-2.5 text-xs font-medium text-foreground shadow-xs transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-pointer"
      >
        <Show when={props.icon}>
          {(ic) => <Icon icon={ic()} size={13} class="text-muted-foreground" />}
        </Show>
        <span>{currentLabel()}</span>
        <Icon
          icon={ChevronDown}
          size={12}
          strokeWidth={2}
          class={cn("text-muted-foreground transition-transform duration-150", open() && "rotate-180")}
        />
      </button>

      <Show when={open()}>
        <div
          role="listbox"
          class={cn(
            "absolute z-50 mt-1.5 min-w-[132px] rounded-md border border-border bg-card p-1 shadow-md",
            props.align === "right" ? "right-0" : "left-0"
          )}
        >
          <For each={props.options}>
            {(opt) => {
              const selected = () => opt.value === props.value;
              return (
                <button
                  type="button"
                  role="option"
                  aria-selected={selected() ? "true" : "false"}
                  onClick={() => {
                    props.onChange(opt.value);
                    setOpen(false);
                  }}
                  class={cn(
                    "flex w-full items-center justify-between gap-2 rounded-sm px-2.5 py-1.5 text-left text-xs transition-colors cursor-pointer",
                    selected()
                      ? "bg-accent font-medium text-foreground"
                      : "text-muted-foreground hover:bg-accent/60 hover:text-foreground"
                  )}
                >
                  <span>{opt.label}</span>
                  <Show when={selected()}>
                    <Icon icon={Check} size={12} strokeWidth={2.2} class="text-primary" />
                  </Show>
                </button>
              );
            }}
          </For>
        </div>
      </Show>
    </div>
  );
}

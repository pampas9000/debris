import { type JSX } from "@solidjs/web";
import { omit } from "solid-js";
import { cn } from "../../lib/utils";

export function Input(props: JSX.InputHTMLAttributes<HTMLInputElement>) {
  const rest = omit(props, "class");
  return (
    <input
      class={cn(
        "flex h-8 w-full rounded-md border border-input bg-card px-2.5 py-1 text-xs text-foreground shadow-xs transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50",
        props.class
      )}
      {...rest}
    />
  );
}

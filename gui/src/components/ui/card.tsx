import { type JSX } from "@solidjs/web";
import { omit } from "solid-js";
import { cn } from "../../lib/utils";

export function Card(props: JSX.HTMLAttributes<HTMLElement>) {
  const rest = omit(props, "class");
  return (
    <section
      class={cn("rounded-lg border border-border bg-card text-card-foreground shadow-xs", props.class)}
      {...rest}
    />
  );
}

export function CardHeader(props: JSX.HTMLAttributes<HTMLDivElement>) {
  const rest = omit(props, "class");
  return <div class={cn("flex items-center justify-between gap-3 px-4 pt-4 pb-2", props.class)} {...rest} />;
}

export function CardTitle(props: JSX.HTMLAttributes<HTMLHeadingElement>) {
  const rest = omit(props, "class");
  return <h2 class={cn("text-[13px] font-semibold leading-none tracking-tight", props.class)} {...rest} />;
}

export function CardContent(props: JSX.HTMLAttributes<HTMLDivElement>) {
  const rest = omit(props, "class");
  return <div class={cn("px-4 pb-4 pt-1", props.class)} {...rest} />;
}

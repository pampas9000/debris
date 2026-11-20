import { type JSX } from "@solidjs/web";
import { omit } from "solid-js";
import { type VariantProps, cva } from "class-variance-authority";
import { cn } from "../../lib/utils";

const alertVariants = cva("flex items-start gap-2.5 rounded-md px-3 py-2.5 text-xs select-text", {
  variants: {
    variant: {
      default: "border border-border bg-card text-foreground",
      destructive: "bg-destructive/10 text-destructive",
      warning: "bg-amber-500/10 text-amber-800 dark:text-amber-300",
      success: "bg-emerald-500/10 text-emerald-800 dark:text-emerald-300",
    },
  },
  defaultVariants: {
    variant: "default",
  },
});

export type AlertProps = JSX.HTMLAttributes<HTMLDivElement> & VariantProps<typeof alertVariants>;

export function Alert(props: AlertProps) {
  const rest = omit(props, "variant", "class");
  return <div role="alert" class={cn(alertVariants({ variant: props.variant }), props.class)} {...rest} />;
}

import { type JSX } from "@solidjs/web";
import { omit } from "solid-js";
import { type VariantProps, cva } from "class-variance-authority";
import { cn } from "../../lib/utils";

const badgeVariants = cva(
  "inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium transition-colors",
  {
    variants: {
      variant: {
        default: "bg-primary/12 text-primary",
        secondary: "bg-secondary text-secondary-foreground",
        outline: "border border-border text-muted-foreground",
        warning: "bg-amber-500/12 text-amber-700 dark:text-amber-400",
        success: "bg-emerald-500/12 text-emerald-700 dark:text-emerald-400",
        destructive: "bg-destructive/12 text-destructive",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
);

export type BadgeProps = JSX.HTMLAttributes<HTMLSpanElement> & VariantProps<typeof badgeVariants>;

export function Badge(props: BadgeProps) {
  const rest = omit(props, "variant", "class");
  return <span class={cn(badgeVariants({ variant: props.variant }), props.class)} {...rest} />;
}

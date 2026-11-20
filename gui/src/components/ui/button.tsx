import { type JSX } from "@solidjs/web";
import { omit } from "solid-js";
import { type VariantProps, cva } from "class-variance-authority";
import { cn } from "../../lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-45 cursor-pointer",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground shadow-xs hover:bg-primary/90",
        secondary: "bg-secondary text-secondary-foreground hover:bg-secondary/80",
        outline: "border border-input bg-card hover:bg-accent hover:text-accent-foreground shadow-xs",
        ghost: "hover:bg-accent hover:text-accent-foreground",
        destructive: "bg-destructive/12 text-destructive hover:bg-destructive/20",
      },
      size: {
        default: "h-8 px-3.5 py-1.5",
        sm: "h-7 rounded-md px-2.5 text-xs",
        lg: "h-9 rounded-md px-4",
        icon: "h-8 w-8",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
);

export type ButtonProps = JSX.ButtonHTMLAttributes<HTMLButtonElement> &
  VariantProps<typeof buttonVariants>;

export function Button(props: ButtonProps) {
  const rest = omit(props, "variant", "size", "class");
  return (
    <button
      class={cn(buttonVariants({ variant: props.variant, size: props.size }), props.class)}
      {...rest}
    />
  );
}

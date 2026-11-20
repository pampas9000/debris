import { Dynamic } from "@solidjs/web";
import { For } from "solid-js";
import type { IconNode } from "lucide";
import { cn } from "../../lib/utils";

export interface IconProps {
  icon: IconNode;
  size?: number;
  strokeWidth?: number;
  class?: string;
}

export function Icon(props: IconProps) {
  const size = () => props.size ?? 15;
  return (
    <svg
      width={size()}
      height={size()}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width={props.strokeWidth ?? 1.75}
      stroke-linecap="round"
      stroke-linejoin="round"
      class={cn("shrink-0", props.class)}
      aria-hidden="true"
    >
      <For each={props.icon}>
        {([tag, attrs]) => <Dynamic component={tag} {...attrs} />}
      </For>
    </svg>
  );
}

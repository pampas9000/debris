import { cn } from "../../lib/utils";

export interface ProgressProps {
  value: number;
  max?: number;
  class?: string;
}

export function Progress(props: ProgressProps) {
  const percent = () => {
    const max = props.max ?? 100;
    if (max <= 0) return 0;
    return Math.min(100, Math.max(0, Math.round((props.value / max) * 100)));
  };

  return (
    <div
      role="progressbar"
      aria-valuemin={0}
      aria-valuemax={props.max ?? 100}
      aria-valuenow={props.value}
      class={cn("h-2 w-full overflow-hidden rounded-full bg-secondary", props.class)}
    >
      <div
        class="h-full rounded-full bg-primary transition-[width] duration-150 ease-out"
        style={{ width: `${percent()}%` }}
      />
    </div>
  );
}

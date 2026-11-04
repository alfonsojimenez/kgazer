import type { ClusterInfo } from "@/lib/api";
import { cn } from "@/lib/utils";

type ClusterStatus = ClusterInfo["status"];

const STATUS_CLASS: Record<ClusterStatus, string> = {
  connected: "bg-emerald-500",
  connecting: "bg-amber-500 animate-pulse",
  disconnected: "bg-red-500",
};

const STATUS_LABEL: Record<ClusterStatus, string> = {
  connected: "Connected",
  connecting: "Connecting",
  disconnected: "Disconnected",
};

export function StatusDot({ status, className }: { status: ClusterStatus; className?: string }) {
  const label = STATUS_LABEL[status];
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      className={cn("inline-block h-2 w-2 shrink-0 rounded-full", STATUS_CLASS[status], className)}
    />
  );
}

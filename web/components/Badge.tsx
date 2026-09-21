import type { Severity } from "@/lib/api";
import type { ReactNode } from "react";

const colors: Record<string, string> = {
  critical: "bg-red-500/15 text-red-300 border-red-500/30",
  error: "bg-orange-500/15 text-orange-300 border-orange-500/30",
  warning: "bg-amber-500/15 text-amber-200 border-amber-500/30",
  info: "bg-sky-500/15 text-sky-300 border-sky-500/30",
  queued: "bg-zinc-500/15 text-zinc-300 border-zinc-500/30",
  running: "bg-brass-500/15 text-brass-300 border-brass-500/30",
  completed: "bg-emerald-500/15 text-emerald-300 border-emerald-500/30",
  failed: "bg-red-500/15 text-red-300 border-red-500/30",
  ok: "bg-emerald-500/15 text-emerald-300 border-emerald-500/30",
  skipped: "bg-zinc-500/15 text-zinc-400 border-zinc-500/30",
  degraded: "bg-amber-500/15 text-amber-200 border-amber-500/30",
  static: "bg-ink-700 text-zinc-300 border-ink-600",
  llm: "bg-violet-500/15 text-violet-200 border-violet-500/30",
};

export default function Badge({ children, kind }: { children: ReactNode; kind: string }) {
  return (
    <span
      className={`inline-flex items-center rounded border px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wide ${colors[kind] || colors.info}`}
    >
      {children}
    </span>
  );
}

export function Sev({ s }: { s: Severity | string }) {
  return <Badge kind={s}>{s}</Badge>;
}

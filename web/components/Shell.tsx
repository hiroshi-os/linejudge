import Link from "next/link";
import type { ReactNode } from "react";

const nav = [
  { href: "/", label: "Dock" },
  { href: "/reviews", label: "Reviews" },
  { href: "/repos", label: "Repos" },
  { href: "/config", label: "Checks" },
  { href: "/eval", label: "Eval" },
];

export default function Shell({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-screen grid grid-cols-[220px_1fr]">
      <aside className="border-r border-ink-700 bg-ink-900/80 px-5 py-6 flex flex-col gap-8">
        <Link href="/" className="block">
          <div className="font-serif text-2xl tracking-tight text-brass-300">linejudge</div>
          <div className="mt-1 font-mono text-[11px] uppercase tracking-[0.18em] text-zinc-500">
            line-level review
          </div>
        </Link>
        <nav className="flex flex-col gap-1">
          {nav.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className="rounded-md px-3 py-2 text-sm text-zinc-300 hover:bg-ink-800 hover:text-white"
            >
              {item.label}
            </Link>
          ))}
        </nav>
        <div className="mt-auto text-[11px] leading-relaxed text-zinc-500 font-mono">
          GitHub App-shaped pipeline.
          <br />
          LLM is a gated stage, not the product.
        </div>
      </aside>
      <main className="min-w-0">{children}</main>
    </div>
  );
}

"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, type Review } from "@/lib/api";
import Badge from "@/components/Badge";

type Fixture = { name: string; repo: string; pr: number; title: string; clean: boolean; labels: number };

export default function ReviewsPage() {
  const [reviews, setReviews] = useState<Review[]>([]);
  const [fixtures, setFixtures] = useState<Fixture[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  async function load() {
    const [r, f] = await Promise.all([api.reviews(), api.fixtures()]);
    setReviews(r);
    setFixtures(f);
  }

  useEffect(() => {
    load().catch((e) => setErr(String(e)));
    const t = setInterval(() => load().catch(() => null), 2000);
    return () => clearInterval(t);
  }, []);

  async function replay(name: string) {
    setBusy(name);
    setErr(null);
    try {
      await api.replay(name);
      await load();
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="px-10 py-8 space-y-8">
      <header>
        <div className="font-mono text-[11px] uppercase tracking-[0.22em] text-brass-400">history</div>
        <h1 className="font-serif text-4xl mt-1">Review runs</h1>
        <p className="mt-2 text-sm text-zinc-400 max-w-2xl">
          Each GitHub <code>pull_request</code> / <code>check_suite</code> delivery becomes a job.
          Replay a golden fixture to watch the pipeline without a registered App.
        </p>
      </header>

      {err && <div className="text-sm text-red-300">{err}</div>}

      <section className="flex flex-wrap gap-2">
        {fixtures.map((f) => (
          <button
            key={f.name}
            onClick={() => replay(f.name)}
            disabled={busy === f.name}
            className="rounded-md border border-ink-600 bg-ink-800 px-3 py-2 text-left text-sm hover:border-brass-500/50 disabled:opacity-50"
          >
            <div className="font-mono text-xs text-brass-300">{f.name}</div>
            <div className="text-zinc-400 text-xs">
              {f.repo}#{f.pr} · {f.labels} labeled
              {f.clean ? " · clean" : ""}
            </div>
          </button>
        ))}
      </section>

      <div className="rounded-lg border border-ink-700 overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-ink-800 text-left text-[11px] uppercase tracking-wider text-zinc-500">
            <tr>
              <th className="px-4 py-2">When</th>
              <th className="px-4 py-2">PR</th>
              <th className="px-4 py-2">Repo</th>
              <th className="px-4 py-2">Status</th>
              <th className="px-4 py-2">Comments</th>
              <th className="px-4 py-2">Fixture</th>
            </tr>
          </thead>
          <tbody>
            {reviews.map((r) => (
              <tr key={r.id} className="border-t border-ink-700 hover:bg-ink-800/40">
                <td className="px-4 py-3 font-mono text-xs text-zinc-500">
                  {new Date(r.created_at).toLocaleString()}
                </td>
                <td className="px-4 py-3">
                  <Link className="text-brass-300 hover:underline" href={`/reviews/${r.id}`}>
                    #{r.pr_number} {r.title}
                  </Link>
                </td>
                <td className="px-4 py-3 font-mono text-xs">{r.repo_full_name}</td>
                <td className="px-4 py-3">
                  <Badge kind={r.status}>{r.status}</Badge>
                </td>
                <td className="px-4 py-3 font-mono">
                  {r.comment_count} / {r.finding_count}
                </td>
                <td className="px-4 py-3 font-mono text-xs text-zinc-500">{r.fixture || "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

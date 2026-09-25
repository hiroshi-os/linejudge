"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, type Review } from "@/lib/api";
import Badge from "@/components/Badge";

export default function HomePage() {
  const [stats, setStats] = useState<Record<string, number>>({});
  const [reviews, setReviews] = useState<Review[]>([]);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    Promise.all([api.stats(), api.reviews()])
      .then(([s, r]) => {
        setStats(s);
        setReviews(r.slice(0, 8));
      })
      .catch((e) => setErr(String(e)));
  }, []);

  return (
    <div className="px-10 py-8 space-y-8">
      <header className="flex items-end justify-between">
        <div>
          <div className="font-mono text-[11px] uppercase tracking-[0.22em] text-brass-400">
            review dock
          </div>
          <h1 className="font-serif text-4xl mt-1">What landed on the bench</h1>
          <p className="mt-2 max-w-xl text-sm text-zinc-400">
            Webhook in, structured comments out. Static checks run on every PR; the LLM stage stays
            fail-closed until you approve it.
          </p>
        </div>
        <Link
          href="/reviews"
          className="rounded-md border border-brass-500/40 bg-brass-500/10 px-3 py-2 text-sm text-brass-300"
        >
          Open history
        </Link>
      </header>

      {err && (
        <div className="rounded border border-red-500/40 bg-red-500/10 px-4 py-3 text-sm text-red-200">
          API unreachable at <code>/v1</code>. Start the Go server on :8080. {err}
        </div>
      )}

      <section className="grid grid-cols-4 gap-3">
        {[
          ["repos", "Connected repos"],
          ["reviews", "Review runs"],
          ["findings", "Findings"],
          ["comments", "Published comments"],
        ].map(([k, label]) => (
          <div key={k} className="rounded-lg border border-ink-700 bg-ink-900 p-4">
            <div className="font-mono text-3xl text-brass-300">{stats[k] ?? 0}</div>
            <div className="mt-1 text-xs uppercase tracking-wider text-zinc-500">{label}</div>
          </div>
        ))}
      </section>

      <section>
        <h2 className="font-serif text-xl mb-3">Recent runs</h2>
        <div className="rounded-lg border border-ink-700 overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-ink-800 text-left text-[11px] uppercase tracking-wider text-zinc-500">
              <tr>
                <th className="px-4 py-2">PR</th>
                <th className="px-4 py-2">Repo</th>
                <th className="px-4 py-2">Status</th>
                <th className="px-4 py-2">Findings</th>
                <th className="px-4 py-2">Event</th>
              </tr>
            </thead>
            <tbody>
              {reviews.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-zinc-500">
                    No reviews yet. Replay a fixture from Reviews.
                  </td>
                </tr>
              )}
              {reviews.map((r) => (
                <tr key={r.id} className="border-t border-ink-700 hover:bg-ink-800/50">
                  <td className="px-4 py-3">
                    <Link href={`/reviews/${r.id}`} className="text-brass-300 hover:underline">
                      #{r.pr_number} {r.title || r.id}
                    </Link>
                  </td>
                  <td className="px-4 py-3 font-mono text-xs text-zinc-400">{r.repo_full_name}</td>
                  <td className="px-4 py-3">
                    <Badge kind={r.status}>{r.status}</Badge>
                  </td>
                  <td className="px-4 py-3 font-mono">{r.finding_count}</td>
                  <td className="px-4 py-3 font-mono text-xs text-zinc-500">{r.event_type}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

"use client";

import { useEffect, useState } from "react";
import { api, type EvalReport } from "@/lib/api";

function pct(n: number) {
  return `${(n * 100).toFixed(1)}%`;
}

export default function EvalPage() {
  const [rep, setRep] = useState<EvalReport | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    api
      .latestEval()
      .then(setRep)
      .catch(() => setRep(null));
  }, []);

  async function run() {
    setBusy(true);
    setErr(null);
    try {
      setRep(await api.runEval());
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="px-10 py-8 space-y-8">
      <header className="flex items-end justify-between">
        <div>
          <div className="font-mono text-[11px] uppercase tracking-[0.22em] text-brass-400">
            harness
          </div>
          <h1 className="font-serif text-4xl mt-1">Eval on golden PRs</h1>
          <p className="mt-2 text-sm text-zinc-400 max-w-2xl">
            Labeled issues in <code>fixtures/*/labels.json</code>. A hit is the same path and a line
            within ±2. These numbers are measured, not advertised.
          </p>
        </div>
        <button
          onClick={run}
          disabled={busy}
          className="rounded border border-brass-500/40 bg-brass-500/10 px-4 py-2 text-sm text-brass-300 disabled:opacity-50"
        >
          {busy ? "Running…" : "Run eval"}
        </button>
      </header>
      {err && <div className="text-sm text-red-300">{err}</div>}
      {!rep && <div className="text-zinc-500 text-sm">No report yet. Run eval.</div>}
      {rep && (
        <>
          <section className="grid grid-cols-4 gap-3">
            {[
              ["Precision", pct(rep.precision)],
              ["Recall / hit-rate", pct(rep.recall)],
              ["F1", pct(rep.f1)],
              ["Precision@5", pct(rep.precision_at_5)],
            ].map(([k, v]) => (
              <div key={k} className="rounded-lg border border-ink-700 bg-ink-900 p-4">
                <div className="font-mono text-3xl text-brass-300">{v}</div>
                <div className="mt-1 text-xs uppercase tracking-wider text-zinc-500">{k}</div>
              </div>
            ))}
          </section>
          <div className="text-xs text-zinc-500 font-mono">
            {rep.hits}/{rep.labeled_issues} labeled issues hit · {rep.findings} findings · clean-PR
            FPs {rep.clean_pr_false_positives} · {rep.generated_at}
          </div>
          <table className="w-full text-sm rounded-lg border border-ink-700 overflow-hidden">
            <thead className="bg-ink-800 text-left text-[11px] uppercase tracking-wider text-zinc-500">
              <tr>
                <th className="px-4 py-2">Fixture</th>
                <th className="px-4 py-2">Labeled</th>
                <th className="px-4 py-2">Findings</th>
                <th className="px-4 py-2">Hits</th>
                <th className="px-4 py-2">P</th>
                <th className="px-4 py-2">R</th>
              </tr>
            </thead>
            <tbody>
              {rep.by_fixture.map((f) => (
                <tr key={f.name} className="border-t border-ink-700">
                  <td className="px-4 py-2 font-mono text-xs">
                    {f.name}
                    {f.clean_pr ? " (clean)" : ""}
                  </td>
                  <td className="px-4 py-2">{f.labeled}</td>
                  <td className="px-4 py-2">{f.findings}</td>
                  <td className="px-4 py-2">{f.hits}</td>
                  <td className="px-4 py-2 font-mono">{f.precision.toFixed(2)}</td>
                  <td className="px-4 py-2 font-mono">{f.recall.toFixed(2)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="text-xs text-zinc-500 max-w-2xl">{rep.notes}</p>
        </>
      )}
    </div>
  );
}

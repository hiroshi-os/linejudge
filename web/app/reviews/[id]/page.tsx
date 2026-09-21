"use client";

import { useEffect, useMemo, useState } from "react";
import { useParams } from "next/navigation";
import { api, type Finding, type ReviewDetail } from "@/lib/api";
import Badge, { Sev } from "@/components/Badge";

export default function ReviewDetailPage() {
  const params = useParams<{ id: string }>();
  const [data, setData] = useState<ReviewDetail | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [active, setActive] = useState<string | null>(null);

  useEffect(() => {
    let stop = false;
    async function tick() {
      try {
        const d = await api.review(params.id);
        if (!stop) setData(d);
        if (d.review.status === "queued" || d.review.status === "running") {
          setTimeout(tick, 400);
        }
      } catch (e) {
        if (!stop) setErr(String(e));
      }
    }
    tick();
    return () => {
      stop = true;
    };
  }, [params.id]);

  const byLine = useMemo(() => {
    const m = new Map<string, Finding[]>();
    for (const f of data?.findings || []) {
      const k = `${f.path}:${f.line}`;
      m.set(k, [...(m.get(k) || []), f]);
    }
    return m;
  }, [data]);

  if (err) return <div className="p-10 text-red-300">{err}</div>;
  if (!data) return <div className="p-10 text-zinc-500">Loading review…</div>;

  const { review, findings, events, packed, comments } = data;

  return (
    <div className="grid grid-cols-[minmax(0,1fr)_340px] min-h-screen">
      <div className="px-8 py-7 border-r border-ink-700 min-w-0">
        <div className="font-mono text-[11px] uppercase tracking-[0.22em] text-brass-400">
          {review.repo_full_name} · {review.event_type}
        </div>
        <h1 className="font-serif text-3xl mt-1">
          #{review.pr_number} {review.title}
        </h1>
        <div className="mt-3 flex flex-wrap gap-2 items-center">
          <Badge kind={review.status}>{review.status}</Badge>
          <span className="font-mono text-xs text-zinc-500">{review.sha.slice(0, 12)}</span>
          <span className="text-xs text-zinc-500">
            {comments.length} GitHub-shaped comments · {findings.length} findings
          </span>
        </div>

        <div className="mt-8 space-y-8">
          {(packed || []).map((file) => (
            <section key={file.path} className="rounded-lg border border-ink-700 overflow-hidden">
              <header className="flex items-center justify-between bg-ink-800 px-4 py-2">
                <div className="font-mono text-sm">{file.path}</div>
                <div className="font-mono text-[11px] text-zinc-500">
                  +{file.added} / −{file.deleted} · {file.language}
                </div>
              </header>
              {file.hunks.map((h, i) => (
                <div key={i} className="font-mono text-[12px] leading-6">
                  <div className="bg-ink-900 px-4 py-1 text-zinc-500">{h.header}</div>
                  {h.lines.map((ln, j) => {
                    const lineNo = ln.new_no || ln.old_no;
                    const hits = byLine.get(`${file.path}:${ln.new_no}`) || [];
                    const kind = ln.kind || " ";
                    const bg =
                      kind === "+" ? "bg-emerald-500/5" : kind === "-" ? "bg-red-500/5" : "";
                    return (
                      <div key={j}>
                        <div className={`grid grid-cols-[48px_20px_1fr] px-2 ${bg}`}>
                          <div className="text-right pr-2 text-zinc-600 select-none">{lineNo || ""}</div>
                          <div className="text-zinc-500">{kind}</div>
                          <pre className="whitespace-pre-wrap break-all text-zinc-200">{ln.text}</pre>
                        </div>
                        {hits.map((f) => (
                          <button
                            key={f.id}
                            onClick={() => setActive(f.id)}
                            className="mx-4 my-2 block w-[calc(100%-2rem)] rounded border border-brass-500/30 bg-ink-800 px-3 py-2 text-left"
                          >
                            <div className="flex items-center gap-2">
                              <Sev s={f.severity} />
                              <Badge kind={f.source}>{f.source}</Badge>
                              <span className="text-xs text-zinc-200">{f.title}</span>
                            </div>
                            <div className="mt-1 text-[11px] text-zinc-400">{f.suggestion}</div>
                          </button>
                        ))}
                      </div>
                    );
                  })}
                </div>
              ))}
            </section>
          ))}
        </div>
      </div>

      <aside className="px-5 py-7 space-y-6 bg-ink-900/40">
        <section>
          <h2 className="font-serif text-lg">Pipeline</h2>
          <ol className="mt-3 space-y-2">
            {events.map((e) => (
              <li key={e.id} className="flex gap-2 items-start">
                <Badge kind={e.status}>{e.status}</Badge>
                <div>
                  <div className="font-mono text-xs">{e.stage}</div>
                  <div className="text-[11px] text-zinc-500">{e.detail}</div>
                </div>
              </li>
            ))}
          </ol>
        </section>
        <section>
          <h2 className="font-serif text-lg">Findings</h2>
          <ul className="mt-3 space-y-2">
            {findings.map((f) => (
              <li
                key={f.id}
                className={`rounded border px-3 py-2 ${active === f.id ? "border-brass-400" : "border-ink-700"}`}
              >
                <div className="flex gap-2 items-center">
                  <Sev s={f.severity} />
                  <span className="text-xs">{f.title}</span>
                </div>
                <div className="mt-1 font-mono text-[11px] text-zinc-500">
                  {f.path}:{f.line} · {f.check_id}
                </div>
              </li>
            ))}
          </ul>
        </section>
      </aside>
    </div>
  );
}

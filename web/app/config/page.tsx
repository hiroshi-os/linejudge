"use client";

import { useEffect, useState } from "react";
import { api, type AppConfig } from "@/lib/api";

const CHECK_COPY: Record<string, string> = {
  secrets: "Regex + entropy-ish assignments for keys, PEMs, PATs, Slack tokens.",
  complexity: "Hunk-local cyclomatic heuristic on added functions.",
  bugs: "SQL sprintf, discarded errors, security TODOs.",
  insecure: "TLS skip-verify, broken hashes, http.Client with no timeout.",
  "js-unsafe": "innerHTML, eval, document.write.",
  "llm-assist": "Optional semantic pass. Fail-closed until Approve is on. Mock provider is deterministic; OpenAI if OPENAI_API_KEY is set.",
};

export default function ConfigPage() {
  const [cfg, setCfg] = useState<AppConfig | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    api.config().then(setCfg).catch((e) => setErr(String(e)));
  }, []);

  async function save() {
    if (!cfg) return;
    try {
      const next = await api.saveConfig(cfg);
      setCfg(next);
      setMsg("Saved. The next queued review uses this config.");
    } catch (e) {
      setErr(String(e));
    }
  }

  if (!cfg) return <div className="p-10 text-zinc-500">{err || "Loading config…"}</div>;

  return (
    <div className="px-10 py-8 space-y-8 max-w-3xl">
      <header>
        <div className="font-mono text-[11px] uppercase tracking-[0.22em] text-brass-400">policy</div>
        <h1 className="font-serif text-4xl mt-1">Checks & LLM gate</h1>
        <p className="mt-2 text-sm text-zinc-400">
          Severity floor, comment cap, and which stages run. Approving the LLM stage is an explicit
          operator action — the pipeline still publishes static findings if the model is down.
        </p>
      </header>

      <section className="grid grid-cols-2 gap-4">
        <label className="text-xs text-zinc-400">
          Minimum severity
          <select
            className="mt-1 block w-full rounded border border-ink-600 bg-ink-800 px-3 py-2 text-sm"
            value={cfg.min_severity}
            onChange={(e) => setCfg({ ...cfg, min_severity: e.target.value as AppConfig["min_severity"] })}
          >
            <option>info</option>
            <option>warning</option>
            <option>error</option>
            <option>critical</option>
          </select>
        </label>
        <label className="text-xs text-zinc-400">
          Max comments / PR
          <input
            type="number"
            className="mt-1 block w-full rounded border border-ink-600 bg-ink-800 px-3 py-2 text-sm"
            value={cfg.max_comments}
            onChange={(e) => setCfg({ ...cfg, max_comments: Number(e.target.value) })}
          />
        </label>
        <label className="text-xs text-zinc-400 col-span-2">
          LLM provider
          <select
            className="mt-1 block w-full rounded border border-ink-600 bg-ink-800 px-3 py-2 text-sm"
            value={cfg.llm_provider}
            onChange={(e) => setCfg({ ...cfg, llm_provider: e.target.value })}
          >
            <option value="mock">mock (deterministic, no key)</option>
            <option value="openai">openai (OPENAI_API_KEY)</option>
          </select>
        </label>
      </section>

      <section className="space-y-3">
        {Object.entries(cfg.checks).map(([id, c]) => (
          <div key={id} className="rounded-lg border border-ink-700 bg-ink-900 p-4">
            <div className="flex items-center justify-between">
              <div>
                <div className="font-mono text-sm text-brass-300">{id}</div>
                <div className="text-xs text-zinc-500 mt-1">{CHECK_COPY[id]}</div>
              </div>
              <label className="text-xs flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={c.enabled}
                  onChange={(e) =>
                    setCfg({
                      ...cfg,
                      checks: { ...cfg.checks, [id]: { ...c, enabled: e.target.checked } },
                    })
                  }
                />
                on
              </label>
            </div>
            <div className="mt-3 flex gap-4 items-center">
              {id !== "llm-assist" && (
                <label className="text-xs text-zinc-400">
                  severity
                  <select
                    className="ml-2 rounded border border-ink-600 bg-ink-800 px-2 py-1"
                    value={c.severity || "warning"}
                    onChange={(e) =>
                      setCfg({
                        ...cfg,
                        checks: { ...cfg.checks, [id]: { ...c, severity: e.target.value as never } },
                      })
                    }
                  >
                    <option>critical</option>
                    <option>error</option>
                    <option>warning</option>
                    <option>info</option>
                  </select>
                </label>
              )}
              {id === "complexity" && (
                <label className="text-xs text-zinc-400">
                  threshold
                  <input
                    type="number"
                    className="ml-2 w-16 rounded border border-ink-600 bg-ink-800 px-2 py-1"
                    value={c.threshold || 12}
                    onChange={(e) =>
                      setCfg({
                        ...cfg,
                        checks: { ...cfg.checks, [id]: { ...c, threshold: Number(e.target.value) } },
                      })
                    }
                  />
                </label>
              )}
              {id === "llm-assist" && (
                <label className="text-xs flex items-center gap-2 text-brass-300">
                  <input
                    type="checkbox"
                    checked={!!c.approve}
                    onChange={(e) =>
                      setCfg({
                        ...cfg,
                        checks: { ...cfg.checks, [id]: { ...c, approve: e.target.checked } },
                      })
                    }
                  />
                  approve LLM stage
                </label>
              )}
            </div>
          </div>
        ))}
      </section>

      <button
        onClick={save}
        className="rounded border border-brass-500/40 bg-brass-500/10 px-4 py-2 text-sm text-brass-300"
      >
        Save policy
      </button>
      {msg && <div className="text-sm text-emerald-300">{msg}</div>}
    </div>
  );
}

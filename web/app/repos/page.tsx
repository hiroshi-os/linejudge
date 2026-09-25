"use client";

import { FormEvent, useEffect, useState } from "react";
import { api, type Repo } from "@/lib/api";

export default function ReposPage() {
  const [repos, setRepos] = useState<Repo[]>([]);
  const [owner, setOwner] = useState("acme");
  const [name, setName] = useState("");
  const [err, setErr] = useState<string | null>(null);

  async function load() {
    setRepos(await api.repos());
  }

  useEffect(() => {
    load().catch((e) => setErr(String(e)));
  }, []);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    try {
      await api.addRepo(owner, name);
      setName("");
      await load();
    } catch (e) {
      setErr(String(e));
    }
  }

  return (
    <div className="px-10 py-8 space-y-8">
      <header>
        <div className="font-mono text-[11px] uppercase tracking-[0.22em] text-brass-400">
          installations
        </div>
        <h1 className="font-serif text-4xl mt-1">Connected repos</h1>
        <p className="mt-2 text-sm text-zinc-400 max-w-2xl">
          In production these come from the GitHub App installation list. The demo seeds{" "}
          <code>acme/payments</code>, <code>acme/gateway</code>, and <code>acme/web-app</code>.
        </p>
      </header>
      {err && <div className="text-sm text-red-300">{err}</div>}
      <form onSubmit={onSubmit} className="flex gap-2 items-end">
        <label className="text-xs text-zinc-400">
          Owner
          <input
            className="mt-1 block rounded border border-ink-600 bg-ink-800 px-3 py-2"
            value={owner}
            onChange={(e) => setOwner(e.target.value)}
          />
        </label>
        <label className="text-xs text-zinc-400">
          Name
          <input
            className="mt-1 block rounded border border-ink-600 bg-ink-800 px-3 py-2"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="billing"
          />
        </label>
        <button className="rounded border border-brass-500/40 bg-brass-500/10 px-4 py-2 text-sm text-brass-300">
          Connect
        </button>
      </form>
      <ul className="grid grid-cols-2 gap-3">
        {repos.map((r) => (
          <li key={r.id} className="rounded-lg border border-ink-700 bg-ink-900 p-4">
            <div className="font-serif text-xl">{r.full_name}</div>
            <div className="mt-2 font-mono text-[11px] text-zinc-500">
              installation {r.installation_id || "—"} · connected {new Date(r.connected_at).toLocaleDateString()}
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}

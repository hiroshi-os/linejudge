export type Severity = "critical" | "error" | "warning" | "info";

export type Repo = {
  id: string;
  owner: string;
  name: string;
  full_name: string;
  installation_id: string;
  connected_at: string;
};

export type Review = {
  id: string;
  repo_id: string;
  repo_full_name: string;
  pr_number: number;
  title: string;
  sha: string;
  base: string;
  head: string;
  event_type: string;
  delivery_id: string;
  fixture?: string;
  status: string;
  error?: string;
  finding_count: number;
  comment_count: number;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  summary?: string;
};

export type Finding = {
  id: string;
  review_id: string;
  check_id: string;
  category: string;
  severity: Severity;
  path: string;
  line: number;
  end_line?: number;
  side: string;
  title: string;
  body: string;
  rationale?: string;
  suggestion?: string;
  confidence: number;
  source: string;
  fingerprint: string;
};

export type PipelineEvent = {
  id: number;
  review_id: string;
  stage: string;
  status: string;
  detail?: string;
  started_at: string;
  finished_at: string;
};

export type PackedLine = {
  kind: string;
  old_no?: number;
  new_no?: number;
  text: string;
};

export type PackedHunk = {
  header: string;
  old_start: number;
  new_start: number;
  lines: PackedLine[];
};

export type PackedFile = {
  path: string;
  language: string;
  added: number;
  deleted: number;
  hunks: PackedHunk[];
};

export type StoredComment = {
  id: string;
  review_id: string;
  finding_id: string;
  path: string;
  line: number;
  side: string;
  body: string;
  github_comment_id?: string;
  published_at: string;
};

export type CheckConfig = {
  enabled: boolean;
  severity?: Severity;
  threshold?: number;
  approve?: boolean;
};

export type AppConfig = {
  min_severity: Severity;
  ignore_paths: string[];
  max_comments: number;
  checks: Record<string, CheckConfig>;
  llm_provider: string;
};

export type FixtureEval = {
  name: string;
  labeled: number;
  findings: number;
  hits: number;
  missed: string[];
  false_positives: string[];
  precision: number;
  recall: number;
  clean_pr: boolean;
};

export type EvalReport = {
  generated_at: string;
  fixtures: number;
  labeled_issues: number;
  findings: number;
  hits: number;
  precision: number;
  recall: number;
  f1: number;
  hit_rate: number;
  precision_at_5: number;
  precision_at_10: number;
  clean_pr_false_positives: number;
  by_fixture: FixtureEval[];
  notes: string;
};

export type ReviewDetail = {
  review: Review;
  findings: Finding[];
  comments: StoredComment[];
  events: PipelineEvent[];
  packed: PackedFile[];
};

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers || {}) },
    cache: "no-store",
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status} ${text}`);
  }
  return res.json() as Promise<T>;
}

export const api = {
  stats: () => req<Record<string, number>>("/v1/stats"),
  meta: () => req<Record<string, unknown>>("/v1/meta"),
  repos: () => req<Repo[]>("/v1/repos"),
  addRepo: (owner: string, name: string) =>
    req<Repo>("/v1/repos", { method: "POST", body: JSON.stringify({ owner, name }) }),
  config: () => req<AppConfig>("/v1/config"),
  saveConfig: (cfg: AppConfig) =>
    req<AppConfig>("/v1/config", { method: "PUT", body: JSON.stringify(cfg) }),
  reviews: () => req<Review[]>("/v1/reviews"),
  review: (id: string) => req<ReviewDetail>(`/v1/reviews/${id}`),
  fixtures: () =>
    req<{ name: string; repo: string; pr: number; title: string; clean: boolean; labels: number }[]>(
      "/v1/fixtures"
    ),
  replay: (name: string) =>
    req<{ ok: boolean; created: boolean; review: Review; fixture: string }>(`/v1/demo/replay/${name}`, {
      method: "POST",
    }),
  runEval: () => req<EvalReport>("/v1/eval/run", { method: "POST" }),
  latestEval: () => req<EvalReport>("/v1/eval/latest"),
};

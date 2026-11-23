import { GitBranch, TriangleAlert } from "lucide-react";
import { Panel, PanelHead, Tag } from "@/components/bits";
import type { Config, Defaults } from "@/lib/settings";
import { EffortSelect, ModelSelect, NumberField, SelectField, TextField } from "./fields";
import { BACKENDS, modelLabel, STRATEGIES } from "./model";

/** EXAMPLES are the commands a repo of a language is usually built and tested with, shown as examples in its empty fields. */
const EXAMPLES: Record<string, { setup: string; build: string; test: string }> = {
  go: { setup: "go mod download", build: "go build ./...", test: "go test ./..." },
  godot: { setup: "dotnet restore", build: "dotnet build", test: "dotnet test" },
  dotnet: { setup: "dotnet restore", build: "dotnet build", test: "dotnet test" },
  csharp: { setup: "dotnet restore", build: "dotnet build", test: "dotnet test" },
  node: { setup: "npm ci", build: "npm run build", test: "npm test" },
  typescript: { setup: "npm ci", build: "npm run build", test: "npm test" },
  python: { setup: "pip install -r requirements.txt", build: "python -m compileall .", test: "pytest" },
  rust: { setup: "", build: "cargo build", test: "cargo test" },
};
const GENERIC = { setup: "make deps", build: "make build", test: "make test" };
const exampleOf = (language: string, key: "setup" | "build" | "test") => `例如 ${EXAMPLES[language.toLowerCase()]?.[key] || GENERIC[key]}`;

/** ConfigForm edits the part of the configuration a person changes: the name, the commands, who writes the code, the review. */
export function ConfigForm({
  config,
  onChange,
  defaults,
  problems,
}: {
  config: Config;
  onChange: (config: Config) => void;
  defaults: Defaults;
  problems: Record<string, string>;
}) {
  const d = defaults.config;
  const backend = config.worker.backend || d.backend;
  const setRepo = (i: number, key: "setup" | "build" | "test", value: string) =>
    onChange({ ...config, repos: config.repos.map((r, j) => (j === i ? { ...r, [key]: value } : r)) });

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-5">
      <Panel>
        <PanelHead title="專案" />
        <div className="grid gap-4 px-5 pb-5 sm:grid-cols-2">
          <TextField label="名稱" value={config.name} error={problems.name} onChange={(name) => onChange({ ...config, name })} hint="寫進給 AI 看的說明檔。" />
          <TextField label="說明" value={config.description} onChange={(description) => onChange({ ...config, description })} placeholder="給看這份設定的人" hint="AWK 不讀它，只是讓人知道這是什麼專案。" />
        </div>
      </Panel>

      <Panel>
        <PanelHead title="分支" sub="只能看，不能在這裡改。" />
        <dl className="grid gap-3 px-5 pb-4 sm:grid-cols-2">
          {[
            { label: "整合分支", value: config.integration_branch, sub: "每個 PR 都合併到這裡。" },
            { label: "發版分支", value: config.release_branch, sub: "發新版本時才合併到這裡。" },
          ].map((b) => (
            <div key={b.label} className="flex items-start gap-3 rounded-lg border border-border px-3 py-2.5">
              <GitBranch className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden />
              <div className="min-w-0">
                <dt className="text-[13px] font-bold text-muted-foreground">{b.label}</dt>
                <dd className="break-all font-mono text-[14px] font-bold">{b.value || "（沒有設定）"}</dd>
                <dd className="text-[12.5px] text-muted-foreground">{b.sub}</dd>
              </div>
            </div>
          ))}
        </dl>
        <p className="border-t border-border px-5 py-3 text-[13px] text-muted-foreground">
          換分支要先在 GitHub 上建好那個分支，再到終端機改 <code className="rounded bg-muted px-1 font-mono text-[12.5px]">.ai/config.yaml</code>，改完跑{" "}
          <code className="rounded bg-muted px-1 font-mono text-[12.5px]">awkit doctor</code> 檢查。
        </p>
      </Panel>

      <Panel>
        <PanelHead title="建置與測試" sub="執行者交 PR 前會跑這些指令，審查會再跑一次測試。開始工作前，建置和測試都要填。" />
        <div className="grid grid-cols-[minmax(0,1fr)] gap-3 px-5 pb-5">
          {config.repos.map((repo, i) => (
            <div key={repo.name} className="min-w-0 rounded-lg border border-border bg-background p-4">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-[15px] font-extrabold">{repo.name}</span>
                <span className="font-mono text-[12.5px] text-muted-foreground">{repo.path}</span>
                {repo.language && <Tag>{repo.language}</Tag>}
                {(!repo.build.trim() || !repo.test.trim()) && (
                  <Tag tone="caution" icon={TriangleAlert} className="ml-auto">
                    {!repo.build.trim() && !repo.test.trim() ? "建置和測試都還沒填" : !repo.build.trim() ? "建置還沒填" : "測試還沒填"}
                  </Tag>
                )}
              </div>
              <div className="mt-3 grid gap-3 lg:grid-cols-3">
                <TextField label="準備（可以不填）" mono value={repo.setup} placeholder={exampleOf(repo.language, "setup")} onChange={(v) => setRepo(i, "setup", v)} hint="建置前先跑，例如安裝套件。" />
                <TextField label="建置" mono value={repo.build} placeholder={exampleOf(repo.language, "build")} onChange={(v) => setRepo(i, "build", v)} />
                <TextField label="測試" mono value={repo.test} placeholder={exampleOf(repo.language, "test")} onChange={(v) => setRepo(i, "test", v)} />
              </div>
            </div>
          ))}
          {config.repos.length === 0 && <p className="text-[14px] text-muted-foreground">設定裡沒有列出任何 repo。</p>}
          <p className="text-[12.5px] text-muted-foreground">改了這些指令，存檔時也會一起更新 CI 的 workflow。</p>
        </div>
      </Panel>

      <Panel>
        <PanelHead title="誰來寫程式" sub="執行者用哪一個 AI 工具。要先在這台電腦裝好並登入，健康檢查會檢查。" />
        <div className="grid gap-4 px-5 pb-5">
          <fieldset className="grid gap-2 sm:grid-cols-2">
            <legend className="sr-only">執行者用的工具</legend>
            {d.backends.map((id) => (
              <label key={id} className="flex cursor-pointer gap-3 rounded-lg border border-border p-3 has-[:checked]:border-foreground/70 has-[:focus-visible]:ring-3 has-[:focus-visible]:ring-ring/50">
                <input
                  type="radio"
                  name="backend"
                  checked={backend === id}
                  onChange={() => onChange({ ...config, worker: { ...config.worker, backend: id } })}
                  className="mt-1 accent-[var(--foreground)]"
                />
                <span className="min-w-0">
                  <span className="block text-[15px] font-bold">
                    {BACKENDS[id]?.label ?? id}
                    {id === d.backend && <span className="ml-1.5 text-[12.5px] font-normal text-muted-foreground">預設</span>}
                  </span>
                  <span className="block text-[13px] text-muted-foreground">{BACKENDS[id]?.sub}</span>
                </span>
              </label>
            ))}
          </fieldset>
          {backend === "claude-code" && (
            <div className="grid gap-4 sm:grid-cols-2">
              <ModelSelect label="Claude Code 用的模型" value={config.worker.model} inherit={`預設（${modelLabel(d.model)}）`} onChange={(model) => onChange({ ...config, worker: { ...config.worker, model } })} />
              <EffortSelect
                label="想多深"
                value={config.worker.effort}
                inherit="預設（Claude CLI 自己決定）"
                efforts={d.efforts}
                onChange={(effort) => onChange({ ...config, worker: { ...config.worker, effort } })}
                hint="越高越仔細，也越慢、越花錢。"
              />
            </div>
          )}
        </div>
      </Panel>

      <Panel>
        <PanelHead title="審查與合併" />
        <div className="grid gap-4 px-5 pb-5 sm:grid-cols-2">
          <NumberField
            label="過關分數"
            hint="審查給 1 到 10 分，達到這個分數 PR 才合併。"
            min={1}
            max={10}
            unit="分（滿分 10）"
            value={config.review.score_threshold}
            fallback={d.score_threshold}
            error={problems.score}
            onChange={(score_threshold) => onChange({ ...config, review: { ...config.review, score_threshold } })}
          />
          <SelectField
            label="合併的方式"
            value={config.review.merge_strategy || d.merge_strategy}
            options={d.strategies.map((s) => ({ value: s, label: STRATEGIES[s] ?? s }))}
            onChange={(merge_strategy) => onChange({ ...config, review: { ...config.review, merge_strategy } })}
          />
        </div>
      </Panel>

      <Panel>
        <PanelHead title="重試與時限" sub="不填就用預設。" />
        <div className="grid gap-4 px-5 pb-5 sm:grid-cols-2 lg:grid-cols-3">
          <NumberField
            label="審查退回幾次後停下來等你"
            hint="同一個 PR 被退回這麼多次，工單就標成「需要你」。"
            unit="次"
            value={config.retries.review_attempts}
            fallback={d.review_attempts}
            error={problems.attempts}
            onChange={(review_attempts) => onChange({ ...config, retries: { ...config.retries, review_attempts } })}
          />
          <NumberField
            label="執行者失敗後再試幾次"
            hint="0 就是不重試。"
            min={0}
            unit="次"
            value={config.retries.worker}
            fallback={d.worker_retries}
            error={problems.retries}
            onChange={(worker) => onChange({ ...config, retries: { ...config.retries, worker } })}
          />
          <NumberField
            label="一個建置或測試指令最多跑多久"
            hint="超過就停掉，算驗證失敗。測試跑得久的專案調高一點。"
            unit="分鐘"
            value={config.verify_minutes}
            fallback={d.verify_minutes}
            error={problems.minutes}
            onChange={(verify_minutes) => onChange({ ...config, verify_minutes })}
          />
        </div>
      </Panel>
    </div>
  );
}

import { GitBranch, TriangleAlert } from "lucide-react";
import { Panel, PanelHead, Tag } from "@/components/bits";
import type { AgentRole, AgentRoleName, Config, Defaults } from "@/lib/settings";
import { EffortSelect, ModelSelect, MoneyField, NumberField, SelectField, TextField } from "./fields";
import { AGENT_ROLES, agentDefaults, agentProfile, BACKENDS, effortLabel, implementerOf, modelLabel, STRATEGIES } from "./model";

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

const AGENT_WORDS: Record<AgentRoleName, { title: string; sub: string }> = {
  reviewer: { title: "審查者", sub: "讀 PR 的改動和測試結果，打分數、寫意見。它只能讀，不能改檔或跑指令。" },
  resolver: { title: "解衝突者", sub: "PR 和整合分支衝突時，只改衝突的檔案；engine 檢查過才推上去。" },
};

/** AgentFields edit one of the engine's agents: empty is the default, which the box says. */
function AgentFields({
  role,
  config,
  onChange,
  defaults,
  problems,
}: {
  role: AgentRoleName;
  config: Config;
  onChange: (config: Config) => void;
  defaults: Defaults["config"];
  problems: Record<string, string>;
}) {
  const set = config.agents[role];
  const update = (change: Partial<AgentRole>) => onChange({ ...config, agents: { ...config.agents, [role]: { ...set, ...change } } });
  const fallback = agentDefaults(role, config.worker, defaults);
  // What an empty model and effort get, on the backend picked.
  const base = agentProfile(role, { ...set, model: "", effort: "" }, config.worker, defaults);
  const backend = base.backend;
  const tool = BACKENDS[backend]?.label ?? backend;
  const runs = agentProfile(role, set, config.worker, defaults);
  const worker = implementerOf(config.worker, defaults);
  const sameModel = role === "reviewer" && runs.backend === worker.backend && runs.model !== "" && runs.model === worker.model;
  const max = defaults.agents.max_minutes[role];
  const words = AGENT_WORDS[role];
  return (
    <div className="grid min-w-0 content-start gap-3 rounded-lg border border-border bg-background p-4" data-agent={role}>
      <div>
        <div className="text-[15px] font-extrabold">{words.title}</div>
        <p className="text-[13px] text-muted-foreground">{words.sub}</p>
      </div>
      <SelectField
        label="用哪一個工具"
        value={set.backend}
        options={[
          { value: "", label: `預設（${BACKENDS[fallback.backend]?.label ?? fallback.backend}）` },
          ...defaults.backends.map((id) => ({ value: id, label: BACKENDS[id]?.label ?? id })),
        ]}
        // Another tool's model and effort have other names: they start from its own.
        onChange={(value) => update({ backend: value, model: "", effort: "" })}
      />
      <div className="grid gap-3 sm:grid-cols-2">
        {backend === "claude-code" ? (
          <ModelSelect label="模型" value={set.model} inherit={`預設（${base.model ? modelLabel(base.model) : "Claude CLI 自己決定"}）`} onChange={(model) => update({ model })} />
        ) : (
          <TextField
            label="模型"
            mono
            value={set.model}
            placeholder={`預設（${base.model || `${tool} 自己決定`}）`}
            onChange={(model) => update({ model })}
            hint={`${tool} 的模型名稱。`}
          />
        )}
        <EffortSelect
          label="想多深"
          value={set.effort}
          inherit={`預設（${base.effort ? effortLabel(base.effort) : `${tool} 自己決定`}）`}
          efforts={backend === "claude-code" ? defaults.efforts : defaults.agents.codex_efforts}
          onChange={(effort) => update({ effort })}
        />
        <NumberField
          label="一次最多跑多久"
          hint={`最多 ${max} 分鐘；超過就停下，算這一步沒做成。`}
          unit="分鐘"
          min={1}
          max={max}
          value={set.minutes}
          fallback={fallback.minutes}
          error={problems[`${role}.minutes`]}
          onChange={(minutes) => update({ minutes })}
        />
        <MoneyField
          label="一次最多花多少"
          hint={backend === "claude-code" ? "空著就是不限。Claude Code 會在這裡自己停下。" : "空著就是不限。跑完才看得到花了多少，超過就算這一步沒做成。"}
          value={set.usd}
          error={problems[`${role}.usd`]}
          onChange={(usd) => update({ usd })}
        />
      </div>
      {sameModel && (
        <p className="flex gap-1.5 text-[13px] leading-snug text-caution-ink">
          <TriangleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden />
          審查者和執行者是同一個模型（{tool} 的 {modelLabel(runs.model)}），審查比較不獨立。換一個模型，審查比較看得出執行者的盲點。
        </p>
      )}
    </div>
  );
}

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

      <Panel data-settings="agents">
        <PanelHead title="審查與解衝突" sub="engine 用它們審 PR，以及解 PR 和整合分支的衝突。空著就用預設，預設跟著上面的執行者。" />
        <div className="grid grid-cols-[minmax(0,1fr)] gap-3 px-5 pb-5 xl:grid-cols-2">
          {AGENT_ROLES.map((role) => (
            <AgentFields key={role} role={role} config={config} onChange={onChange} defaults={d} problems={problems} />
          ))}
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

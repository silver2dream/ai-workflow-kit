import { useEffect, useMemo, useRef, useState } from "react";
import { Info, Loader2, RefreshCw, Save, TriangleAlert, Undo2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { PageHead, Problem } from "@/components/bits";
import { useProject } from "@/app/project";
import type { ApiError } from "@/lib/api";
import { useResource } from "@/lib/resource";
import { settings, type Busy, type Config, type SettingsView } from "@/lib/settings";
import { cn } from "@/lib/utils";
import { ApplyProgress } from "./ApplyProgress";
import { ConfigForm } from "./ConfigForm";
import { configProblems, editable, rosterProblems, same, sendable, type DraftRoster } from "./model";
import { RosterForm } from "./RosterForm";

/** What a person changed and has not saved, by project: it is kept while they look at another page. */
const unsaved = new Map<string, { version: string; roster: DraftRoster | null; config: Config | null }>();

/** A change done this long ago is no longer shown at the top of the page. */
const SHOW_DONE_FOR = 60 * 60 * 1000;

function busyText(busy: Busy): string {
  switch (busy.kind) {
    case "run":
      return "專案裡有工作正在跑，換設定會讓它做到一半換規則。等它停了再存。";
    case "meeting":
      return `規劃「${busy.plan}」的議會正在開會，等它開完再存。`;
    case "approval":
      return `規劃「${busy.plan}」正在核可，等它做完再存。`;
    default:
      return "上一次的修改還在套用，等它做完再改。";
  }
}

function Banner({ tone, icon: Icon, children }: { tone: "caution" | "info"; icon: typeof Info; children: React.ReactNode }) {
  return (
    <div className={cn("flex gap-2.5 rounded-lg border-l-4 px-4 py-3 text-[14px] leading-relaxed", tone === "caution" ? "border-caution bg-caution/10" : "border-ring bg-ring/5")}>
      <Icon className={cn("mt-0.5 size-4 shrink-0", tone === "caution" ? "text-caution-ink" : "text-ring")} aria-hidden />
      <div className="min-w-0">{children}</div>
    </div>
  );
}

function Original({ text }: { text: string }) {
  return (
    <details className="mt-1.5 text-[13px]">
      <summary className="cursor-pointer font-bold text-muted-foreground">原文</summary>
      <pre className="mt-1 whitespace-pre-wrap break-words rounded-md bg-muted px-3 py-2 font-mono text-[12px] text-ink2">{text}</pre>
    </details>
  );
}

/**
 * SettingsPage changes the council's roster and the part of the
 * configuration a person changes. A save does not write the files here: it
 * goes to the integration branch through a pull request merged once its
 * checks pass, and the project folder is brought up to it after.
 */
export function SettingsPage() {
  const { project, status, meetingMoved, settingsMoved } = useProject();
  const view = useResource(() => settings.get(project.id), [project.id]);
  const reload = view.reload;
  useEffect(() => {
    reload();
  }, [settingsMoved, meetingMoved, status.data?.run.state, reload]);

  const data = view.data;
  const kept = unsaved.get(project.id);
  const [roster, setRoster] = useState<DraftRoster | null>(kept?.roster ?? null);
  const [config, setConfig] = useState<Config | null>(kept?.config ?? null);
  const [tab, setTab] = useState<"roster" | "config">("roster");
  const [saving, setSaving] = useState(false);
  const [retrying, setRetrying] = useState(false);
  const [failure, setFailure] = useState<{ code?: string; message: string } | null>(null);
  /** What was sent, shown while it is on its way, and given back to edit when it does not get there. */
  const [sent, setSent] = useState<{ roster: DraftRoster | null; config: Config | null } | null>(null);

  // What the files hold, as the forms edit it; made again only when the files change.
  const base = useMemo(() => (data ? { version: data.version, roster: data.roster ? editable(data.roster) : null, config: data.config } : null), [data?.version]); // eslint-disable-line react-hooks/exhaustive-deps

  // What was changed for files that have changed since is not kept.
  useEffect(() => {
    if (base && kept && kept.version !== base.version) {
      unsaved.delete(project.id);
      setRoster(null);
      setConfig(null);
    }
  }, [base]); // eslint-disable-line react-hooks/exhaustive-deps

  const applying = data?.apply?.state === "working";
  const lastState = useRef(data?.apply?.state);
  useEffect(() => {
    const now = data?.apply?.state;
    if (lastState.current === "working" && now && now !== "working") {
      if (now === "done") {
        setRoster(null);
        setConfig(null);
        toast.success("設定生效了", { description: data?.apply?.unchanged ? "GitHub 上本來就是這樣。" : "已經合併進整合分支。" });
      } else if (sent) {
        // Given back to be put right and saved again.
        setRoster(sent.roster);
        setConfig(sent.config);
      }
      setSent(null);
    }
    lastState.current = now;
  }, [data?.apply?.state]); // eslint-disable-line react-hooks/exhaustive-deps

  const shownRoster = (applying && sent?.roster) || roster || base?.roster || null;
  const shownConfig = (applying && sent?.config) || config || base?.config || null;
  const rosterChanged = !!roster && !!base?.roster && !same(sendable(roster), sendable(base.roster));
  const configChanged = !!config && !!base?.config && !same(config, base.config);
  const changed = rosterChanged || configChanged;

  useEffect(() => {
    if (!base) return;
    if (roster || config) unsaved.set(project.id, { version: base.version, roster, config });
    else unsaved.delete(project.id);
  }, [roster, config, base, project.id]);

  useEffect(() => {
    if (!changed) return;
    const warn = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [changed]);

  const rProblems = useMemo(() => (shownRoster ? rosterProblems(shownRoster) : { whole: [], roles: {}, count: 0 }), [shownRoster]);
  const cProblems = useMemo(() => (shownConfig ? configProblems(shownConfig, data?.defaults.config) : { fields: {}, count: 0 }), [shownConfig, data?.defaults.config]);
  const problemCount = (rosterChanged ? rProblems.count : 0) + (configChanged ? cProblems.count : 0);

  const blocked = !data
    ? "讀取中…"
    : applying
      ? "上一次的修改還在套用。"
      : data.busy
        ? busyText(data.busy)
        : !data.origin
          ? "這個專案沒有連到 GitHub，存不了。"
          : problemCount > 0
            ? `還有 ${problemCount} 個地方要改。`
            : "";

  const save = async () => {
    if (!data || !changed || blocked) return;
    setSaving(true);
    setFailure(null);
    try {
      await settings.save(project.id, data.version, rosterChanged && roster ? sendable(roster) : undefined, configChanged ? (config ?? undefined) : undefined);
      setSent({ roster: rosterChanged ? roster : null, config: configChanged ? config : null });
      toast.success("開始套用設定", { description: "會開一個 PR，CI 過了就合併進整合分支。關掉工作台也會繼續。" });
      reload();
    } catch (e) {
      const err = e as ApiError;
      setFailure({ code: err.code, message: err.message });
    } finally {
      setSaving(false);
    }
  };

  const retry = async () => {
    setRetrying(true);
    try {
      await settings.retry(project.id);
      toast.success("再試一次了");
      reload();
    } catch (e) {
      toast.error("沒辦法再試", { description: (e as Error).message });
    } finally {
      setRetrying(false);
    }
  };

  const discard = () => {
    setRoster(null);
    setConfig(null);
    setFailure(null);
  };

  const showApply = data?.apply && (data.apply.state !== "done" || Date.now() - Date.parse(data.apply.updated) < SHOW_DONE_FOR);

  return (
    <>
      <PageHead
        title="設定"
        sub="議會由哪些角色組成，以及專案怎麼建置、測試、審查。存檔後，工作台會把修改開成 PR，CI 過了就合併進整合分支，再更新你的專案資料夾；不用自己 commit。"
      />

      {view.error && !data && (
        <Problem title="讀不到設定">
          {view.error.message}
          <div className="mt-2">
            <Button variant="outline" size="sm" onClick={reload}>
              <RefreshCw aria-hidden /> 再讀一次
            </Button>
          </div>
        </Problem>
      )}
      {!data && !view.error && (
        <div className="grid gap-4">
          <Skeleton className="h-10 w-72" />
          <Skeleton className="h-48" />
          <Skeleton className="h-64" />
        </div>
      )}

      {data && (
        <div className="grid grid-cols-[minmax(0,1fr)] gap-4">
          {showApply && data.apply && <ApplyProgress record={data.apply} onRetry={retry} onReload={discard} busy={retrying} retryBlocked={data.busy && data.busy.kind !== "settings" ? busyText(data.busy) : undefined} />}
          {data.busy && data.busy.kind !== "settings" && (
            <Banner tone="caution" icon={TriangleAlert}>
              現在可以改，但還不能存：{busyText(data.busy)}
            </Banner>
          )}
          {!data.origin && (
            <Banner tone="caution" icon={TriangleAlert}>
              這個專案沒有連到 GitHub（沒有叫 origin 的 remote）。設定要透過 PR 合併進整合分支，所以這裡存不了。
            </Banner>
          )}

          <Tabs value={tab} onValueChange={(v) => setTab(v as "roster" | "config")}>
            <TabsList aria-label="設定的種類">
              <TabsTrigger value="roster" data-tour="settings-tab-roster">
                議會名單
                {rosterChanged && <span className="size-2 rounded-full bg-live" aria-label="有修改" />}
                {rosterChanged && rProblems.count > 0 && <span className="rounded bg-destructive/10 px-1 text-[12px] text-destructive tnum">{rProblems.count}</span>}
              </TabsTrigger>
              <TabsTrigger value="config">
                專案設定
                {configChanged && <span className="size-2 rounded-full bg-live" aria-label="有修改" />}
                {configChanged && cProblems.count > 0 && <span className="rounded bg-destructive/10 px-1 text-[12px] text-destructive tnum">{cProblems.count}</span>}
              </TabsTrigger>
            </TabsList>

            <TabsContent value="roster" className="grid grid-cols-[minmax(0,1fr)] gap-4">
              {data.roster_missing && (
                <Banner tone="info" icon={Info}>
                  這個專案還沒有議會名單。下面是第一次開規劃會議時會用的那一份；存檔就會把它加進專案。
                </Banner>
              )}
              {!data.roster_missing && data.roster_problem && data.roster && (
                <Banner tone="caution" icon={TriangleAlert}>
                  名單現在有問題，議會開不了會。在下面改好再存。
                  <Original text={data.roster_problem} />
                </Banner>
              )}
              {shownRoster ? (
                <fieldset disabled={applying} className="min-w-0">
                  <RosterForm roster={shownRoster as DraftRoster} defaults={data.defaults} problems={rProblems} onChange={setRoster} />
                </fieldset>
              ) : (
                <Problem title="讀不了議會名單">
                  {data.roster_problem}。名單檔 .ai/roster.yaml 不是有效的 YAML，要在終端機修好。
                </Problem>
              )}
            </TabsContent>

            <TabsContent value="config" className="grid grid-cols-[minmax(0,1fr)] gap-4">
              {data.config_problem && data.config && (
                <Banner tone="caution" icon={TriangleAlert}>
                  設定檔現在有問題，工作開不了，這一頁也存不了：要在終端機改 <code className="rounded bg-muted px-1 font-mono text-[13px]">.ai/config.yaml</code>，再跑{" "}
                  <code className="rounded bg-muted px-1 font-mono text-[13px]">awkit doctor</code> 檢查。
                  <Original text={data.config_problem} />
                </Banner>
              )}
              {shownConfig ? (
                <fieldset disabled={applying} className="min-w-0">
                  <ConfigForm config={shownConfig} defaults={data.defaults} problems={configChanged ? cProblems.fields : {}} onChange={setConfig} />
                </fieldset>
              ) : (
                <Problem title="讀不了設定檔">{data.config_problem}</Problem>
              )}
            </TabsContent>
          </Tabs>

          <SaveBar
            data={data}
            changed={changed}
            what={[rosterChanged && "議會名單", configChanged && "專案設定"].filter(Boolean) as string[]}
            blocked={blocked}
            saving={saving}
            failure={failure}
            onSave={save}
            onDiscard={discard}
            onReload={() => {
              discard();
              reload();
            }}
          />
        </div>
      )}
    </>
  );
}

function SaveBar({
  data,
  changed,
  what,
  blocked,
  saving,
  failure,
  onSave,
  onDiscard,
  onReload,
}: {
  data: SettingsView;
  changed: boolean;
  what: string[];
  blocked: string;
  saving: boolean;
  failure: { code?: string; message: string } | null;
  onSave: () => void;
  onDiscard: () => void;
  onReload: () => void;
}) {
  if (!changed && !failure) return null;
  return (
    <div className="sticky bottom-0 z-10 -mx-1 rounded-t-xl border border-b-0 border-border bg-card/95 px-4 py-3 shadow-[0_-6px_18px_-12px_rgb(0_0_0/0.35)] backdrop-blur">
      {failure && (
        <div role="alert" className="mb-3 rounded-lg border-l-4 border-destructive bg-destructive/5 px-4 py-2.5 text-[14px]">
          {failure.code === "changed" ? (
            <>
              <div className="font-bold">設定檔在你打開這頁之後被改過了</div>
              <p className="text-ink2">可能是終端機或別的程式改的。重新載入後再改一次；這頁上的修改會不見。</p>
              <Button variant="outline" size="sm" className="mt-2" onClick={onReload}>
                <RefreshCw aria-hidden /> 重新載入設定
              </Button>
            </>
          ) : failure.code === "invalid" ? (
            <>
              <div className="font-bold">這樣的設定用不了</div>
              <p className="text-ink2">議會或工作會因此開不了，所以沒有送出。看原文，改好再存。</p>
              <Original text={failure.message} />
            </>
          ) : (
            <>
              <div className="font-bold">沒有送出</div>
              <p className="text-ink2">{failure.code === "busy" ? "現在不能存：有工作、會議或核可正在進行。" : "工作台沒辦法開始套用。"}</p>
              <Original text={failure.message} />
            </>
          )}
        </div>
      )}
      <div className="flex flex-wrap items-center gap-3">
        <div className="min-w-0 flex-1 text-[14px]">
          {changed ? (
            <>
              <span className="font-bold">改了{what.join("和")}，還沒存</span>
              <span className="block text-[13px] text-muted-foreground">{blocked || `存檔會開 PR 合併進 ${data.config?.integration_branch || "整合分支"}，之後更新你的專案資料夾。`}</span>
            </>
          ) : (
            <span className="text-muted-foreground">沒有要存的修改。</span>
          )}
        </div>
        {changed && (
          <>
            <Button variant="outline" onClick={onDiscard} disabled={saving}>
              <Undo2 aria-hidden /> 不改了
            </Button>
            <Button onClick={onSave} disabled={!!blocked || saving}>
              {saving ? <Loader2 className="animate-spin" aria-hidden /> : <Save aria-hidden />} 存檔並送出
            </Button>
          </>
        )}
      </div>
    </div>
  );
}

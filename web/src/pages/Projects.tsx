import { useState } from "react";
import { FolderPlus, FolderX, TriangleAlert } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Empty, PageHead, Panel, PanelHead } from "@/components/bits";
import { api, type ProjectView, type Session } from "@/lib/api";
import { href } from "@/lib/router";

/** Projects are the folders the person opens in the workbench; taking one out does not touch the folder. */
export function ProjectsPage({ session, onChanged }: { session: Session; onChanged: () => void }) {
  const [path, setPath] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [removing, setRemoving] = useState<ProjectView | null>(null);

  async function add(event: React.FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const added = await api.addProject(path.trim());
      toast.success(`加入了 ${added.name}`);
      setPath("");
      onChanged();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function remove(project: ProjectView) {
    try {
      await api.removeProject(project.id);
      toast.success(`${project.name} 移出工作台了，資料夾沒有動`);
      onChanged();
    } catch (e) {
      toast.error("移不出來", { description: (e as Error).message });
    } finally {
      setRemoving(null);
    }
  }

  return (
    <>
      <PageHead title="你的專案" sub="工作台在這台電腦上跑，只聽這台電腦。專案就是你電腦上用過 awkit init 的資料夾。" />
      <div className="grid gap-5 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
        <Panel>
          <PanelHead title="在工作台裡的專案" />
          {session.projects.length === 0 ? (
            <Empty icon={FolderPlus} title="還沒有專案">在右邊貼上專案資料夾的路徑。也可以在專案資料夾裡執行 awkit ui，它會自己加進來。</Empty>
          ) : (
            <ul>
              {session.projects.map((p) => (
                <li key={p.id} className="flex flex-wrap items-center gap-3 border-t border-border px-5 py-3.5 first:border-t-0">
                  <span className="grid size-10 shrink-0 place-items-center rounded-lg bg-rail text-[16px] font-black text-white">{p.name.slice(0, 1).toUpperCase()}</span>
                  <div className="min-w-0 flex-1">
                    <div className="text-[15px] font-bold">{p.name}</div>
                    <div className="truncate font-mono text-[12.5px] text-muted-foreground">{p.path}</div>
                    {p.problem && (
                      <div className="mt-1 flex items-center gap-1.5 text-[13px] text-caution-ink">
                        <TriangleAlert className="size-3.5" aria-hidden />
                        {p.problem}
                      </div>
                    )}
                  </div>
                  {!p.problem && (
                    <Button asChild size="sm">
                      <a href={href({ project: p.id, page: "overview", rest: [] })}>打開</a>
                    </Button>
                  )}
                  <Button size="sm" variant="ghost" onClick={() => setRemoving(p)}>
                    <FolderX aria-hidden /> 移出
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </Panel>
        <Panel>
          <PanelHead title="加入專案" sub="貼上資料夾的完整路徑" />
          <form onSubmit={add} className="grid gap-3 px-5 pb-5">
            <Label htmlFor="project-path">資料夾</Label>
            <Input id="project-path" value={path} onChange={(e) => setPath(e.target.value)} placeholder="例如 D:\games\rally" className="font-mono" autoComplete="off" />
            {error && (
              <p role="alert" className="text-[13.5px] text-destructive">
                {error}
              </p>
            )}
            <Button type="submit" disabled={busy || !path.trim()}>
              <FolderPlus aria-hidden /> 加入
            </Button>
            <p className="text-[13px] text-muted-foreground">還沒設定過 AWK 的資料夾，要先在那裡執行 awkit init。新專案精靈會在之後的版本加上。</p>
          </form>
        </Panel>
      </div>
      <Dialog open={removing !== null} onOpenChange={(o) => !o && setRemoving(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>把 {removing?.name} 移出工作台？</DialogTitle>
            <DialogDescription>只是不在這個清單裡，資料夾和裡面的東西都不會動。之後可以再加回來。</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRemoving(null)}>
              不要
            </Button>
            <Button onClick={() => removing && remove(removing)}>移出</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

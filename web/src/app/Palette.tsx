import { MapIcon, RefreshCw, SunMoon, Ticket as TicketIcon } from "lucide-react";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { useProject } from "@/app/project";
import { PAGES } from "@/app/pages";
import { useTicketSheet } from "@/app/ticket-sheet";
import { useOnboarding } from "@/onboarding/state";
import { useAfterClose } from "@/lib/after-close";
import { href, navigate } from "@/lib/router";
import { useTheme } from "@/lib/theme";

/** Palette finds a page or a ticket by its name, and does a few things by name. */
export function Palette({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { project, tickets, refreshTickets } = useProject();
  const { open: openTicket } = useTicketSheet();
  const { startTour } = useOnboarding();
  const [theme, toggleTheme] = useTheme();
  const { later, onCloseAutoFocus } = useAfterClose();
  const run = (action: () => void) => () => {
    later(action);
    onOpenChange(false);
  };
  const list = tickets.data?.tickets?.open ?? [];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="top-1/3 translate-y-0 overflow-hidden rounded-xl! p-0 sm:max-w-[600px]" onCloseAutoFocus={onCloseAutoFocus}>
        <DialogHeader className="sr-only">
          <DialogTitle>搜尋或跳到別頁</DialogTitle>
          <DialogDescription>找頁面、工單，或做一件事</DialogDescription>
        </DialogHeader>
        <Command>
          <CommandInput placeholder="找頁面、工單，或輸入動作" />
          <CommandList>
            <CommandEmpty>找不到。</CommandEmpty>
            <CommandGroup heading="頁面">
              {PAGES.map((p) => (
                <CommandItem key={p.page} value={`page ${p.title}`} keywords={[p.page]} onSelect={run(() => navigate(href({ project: project.id, page: p.page, rest: [] })))}>
                  <p.icon aria-hidden />
                  {p.title}
                </CommandItem>
              ))}
            </CommandGroup>
            {list.length > 0 && (
              <CommandGroup heading="工單">
                {list.map((t) => (
                  <CommandItem key={t.number} value={`ticket ${t.number} ${t.title}`} keywords={[`#${t.number}`]} onSelect={run(() => openTicket(t.number))}>
                    <TicketIcon aria-hidden />
                    <span className="tnum">工單 #{t.number}</span>
                    <span className="truncate text-muted-foreground">{t.title}</span>
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
            <CommandGroup heading="動作">
              <CommandItem value="重新問 GitHub refresh" onSelect={run(refreshTickets)}>
                <RefreshCw aria-hidden />
                重新問 GitHub
              </CommandItem>
              <CommandItem value="重看導覽 tour" onSelect={run(() => startTour(0))}>
                <MapIcon aria-hidden />
                重看導覽
              </CommandItem>
              <CommandItem value="切換深淺色 theme dark light" onSelect={run(toggleTheme)}>
                <SunMoon aria-hidden />
                {theme === "dark" ? "換成淺色" : "換成深色"}
              </CommandItem>
            </CommandGroup>
          </CommandList>
        </Command>
      </DialogContent>
    </Dialog>
  );
}

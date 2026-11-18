import { Hand, HeartPulse, History, Kanban, MapIcon, type LucideIcon } from "lucide-react";

export interface PageInfo {
  page: string;
  title: string;
  icon: LucideIcon;
  /** The heading of the side bar's group it starts. */
  group?: string;
}

/** The pages of a project, in the order of the side bar. */
export const PAGES: PageInfo[] = [
  { page: "overview", title: "總覽", icon: MapIcon },
  { page: "inbox", title: "需要你處理", icon: Hand },
  { page: "board", title: "工作看板", icon: Kanban, group: "工作" },
  { page: "activity", title: "活動紀錄", icon: History },
  { page: "health", title: "健康檢查", icon: HeartPulse, group: "這台電腦" },
];

export const pageTitle = (page: string) => PAGES.find((p) => p.page === page)?.title ?? "總覽";

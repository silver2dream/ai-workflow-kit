import { GraduationCap, Hand, HeartPulse, History, Kanban, MapIcon, Play, Users, type LucideIcon } from "lucide-react";

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
  { page: "plans", title: "規劃與議會", icon: Users, group: "工作" },
  { page: "board", title: "工作看板", icon: Kanban },
  { page: "run", title: "執行", icon: Play },
  { page: "activity", title: "活動紀錄", icon: History },
  { page: "lessons", title: "教訓", icon: GraduationCap },
  { page: "health", title: "健康檢查", icon: HeartPulse, group: "這台電腦" },
];

export const pageTitle = (page: string) => PAGES.find((p) => p.page === page)?.title ?? "總覽";

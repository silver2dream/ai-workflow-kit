/**
 * A step of the tour: the page it shows, what on that page it points at, and
 * what it says. targets are tried in order; the first is the best, the others
 * stand in when the project has nothing to show there yet (a board with no
 * stuck ticket has no yellow card to point at).
 */
export interface TourStep {
  page: string;
  targets: string[];
  title: string;
  body: string;
}

export const TOUR: TourStep[] = [
  {
    page: "overview",
    targets: ["linemap"],
    title: "每張工單走到哪一站",
    body: "工作像列車沿著這條線走：需求、議會、工單、寫程式、審查、合併。每個小牌子是一張工單，點它看經過。黃色岔出去的，是卡住、等你的。",
  },
  {
    page: "overview",
    targets: ["attention"],
    title: "黃色代表要你出手",
    body: "只有你能決定的事會出現在這條帶子上，擋住工作的排最前面。整個工作台只有這種事用黃色；沒有事要你處理時，它只是一行安靜的字。",
  },
  {
    page: "plans",
    targets: ["plans", "newplan"],
    title: "想做新東西，從規劃開始",
    body: "寫下你想要什麼，議會的角色會輪流發言、互相質疑，由負責裁定的角色拍板，寫好文件，再擬出工單給你核可。開會時你隨時可以插話或暫停；關掉工作台，會也照樣開。",
  },
  {
    page: "board",
    targets: ["ticketcard", "board"],
    title: "工單和 PR 不一樣",
    body: "工單寫「要做什麼」，PR 是照著工單改好的「程式」。兩者在 GitHub 上共用編號，所以畫面上一律寫清楚是「工單 #306」還是「PR #313」。點開卡片看它的經過。",
  },
  {
    page: "run",
    targets: ["runcontrols"],
    title: "開工、停工都在這裡",
    body: "按「開始工作」，engine 就把工單一張一張交給執行者；要停，可以等手上這一步做完再停，也可以立刻停。下面看得到每張工單走到哪，和執行者現在在說什麼。關掉工作台，工作照樣繼續。",
  },
  {
    page: "activity",
    targets: ["activity"],
    title: "每一步都有紀錄",
    body: "主控、執行者、審查做的每一件事都記在這裡。出問題時從這裡往回找；只想看出錯和警告，就選「只看要注意的」。",
  },
  {
    page: "health",
    targets: ["health"],
    title: "開工前先檢查",
    body: "GitHub、Claude Code、設定、分支都準備好了嗎？之前的工作有沒有留下沒收拾的東西？和終端機的 awkit doctor 是同一套檢查。",
  },
  {
    page: "overview",
    targets: ["help"],
    title: "需要時再找我",
    body: "這裡可以重看導覽、打開上手清單、查名詞。你看到哪一步只記在這台電腦，下次打開會從那裡接著。",
  },
];

/** A thing of the checklist: done once the person did it, wherever they did it. */
export interface Task {
  key: string;
  title: string;
  sub: string;
  cta: string;
  /** The page it is done on; none for the tour, which starts where it is. */
  page?: string;
}

export const TASKS: Task[] = [
  { key: "tour", title: "認識工作台", sub: "跟著導覽走一遍，約 2 分鐘", cta: "開始導覽" },
  { key: "health", title: "檢查這台電腦", sub: "GitHub 與設定準備好了嗎", cta: "去檢查", page: "health" },
  { key: "plan", title: "看議會怎麼規劃", sub: "開一個新的規劃，或打開一份看議會怎麼討論", cta: "去看規劃", page: "plans" },
  { key: "ticket", title: "打開一張工單", sub: "看它的經過、在等誰、GitHub 上的工單與 PR", cta: "去看板", page: "board" },
  { key: "activity", title: "看活動紀錄", sub: "每一步誰做的、結果如何", cta: "去看", page: "activity" },
];

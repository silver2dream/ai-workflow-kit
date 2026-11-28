// What the guardrails say, in words a person who does not write code reads.
// The server says each verdict in English with a rule id; the id is what is
// put in words here, and a rule this page does not know keeps the server's
// own words.

const RULES: Record<string, string> = {
  // Injection rules: what an agent or another person wrote that tries to
  // give the AI orders.
  "injection-rules/1#override-en": "叫讀的人忽略或推翻原本的指示",
  "injection-rules/1#override-zh": "叫讀的人忽略或推翻原本的指示",
  "injection-rules/1#new-instructions": "宣稱有「新的指示」要讀的人照做",
  "injection-rules/1#chat-template": "假裝成和 AI 對話的格式",
  "injection-rules/1#fake-tool-call": "假裝成 AI 的工具呼叫",
  "injection-rules/1#exfil-command": "一段會把密鑰或憑證送到外面的指令",
  "injection-rules/1#role-switch-en": "要讀的人換一個身分",
  "injection-rules/1#role-switch-zh": "要讀的人換一個身分",
  "injection-rules/1#prompt-probe": "要讀的人說出它收到的指示",
  "injection-rules/1#credentials-request": "要讀的人讀出或送出密鑰、憑證",
  "injection-rules/1#awk-internals": "提到 AWK 流程自己的指令或檔案",
  "injection-rules/1#exfil-markdown": "一張圖片，它的網址會把資料帶到外面",
  "injection-rules/1#encoded-blob": "一大段編碼過的內容，可能藏著文字",
  // What is about to be written to GitHub.
  "outbound/1#github-token": "要寫到 GitHub 的內容裡有 GitHub 的 token",
  "outbound/1#github-pat": "要寫到 GitHub 的內容裡有 GitHub 的存取權杖",
  "outbound/1#anthropic-key": "要寫到 GitHub 的內容裡有 Anthropic 的金鑰",
  "outbound/1#openai-key": "要寫到 GitHub 的內容裡有 OpenAI 的金鑰",
  "outbound/1#aws-access-key": "要寫到 GitHub 的內容裡有 AWS 的金鑰",
  "outbound/1#private-key": "要寫到 GitHub 的內容裡有私鑰",
  "outbound/1#slack-token": "要寫到 GitHub 的內容裡有 Slack 的 token",
  "outbound/1#google-api-key": "要寫到 GitHub 的內容裡有 Google 的金鑰",
  "outbound/1#jwt": "要寫到 GitHub 的內容裡有登入用的 token（JWT）",
  "outbound/1#card-number": "要寫到 GitHub 的內容裡有信用卡號",
  "outbound/1#tw-national-id": "要寫到 GitHub 的內容裡有身分證字號",
  // What a Worker's change does that a person would want to look at.
  "diff-signals/1#network": "變更裡新增了連到外面的呼叫",
  "diff-signals/1#dynamic-exec": "變更裡新增了執行時才決定要跑的指令或程式",
  "diff-signals/1#install-script": "變更裡新增了安裝套件時會自動執行的腳本",
  "diff-signals/1#ci-build": "變更改了建置、測試或發版的設定",
  "diff-signals/1#dependency": "變更改了專案用到的套件",
  "diff-signals/1#binary": "變更新增或改了二進位檔",
  "diff-signals/1#mass-deletion": "變更刪掉了大量的程式",
  // What an agent tried to do with its tools.
  "tool-policy/1#merge": "想自己合併 PR（那是審查通過之後的事）",
  "tool-policy/1#issue-create": "想自己開工單",
  "tool-policy/1#network": "想連到外面",
  "tool-policy/1#credentials": "想讀放憑證的地方",
  "tool-policy/1#environment": "想印出所有環境變數",
  "tool-policy/1#secret-variable": "想讀放密鑰的環境變數",
  "tool-policy/1#push-protected": "想推到整合或發版分支",
  "tool-policy/1#push-force": "想強制推送",
  "tool-policy/1#push-delete": "想刪掉遠端的分支",
  "tool-policy/1#push-wide": "想一次推送所有分支",
  "tool-policy/1#git-files": "想改 git 自己的檔案",
  "tool-policy/1#control-files": "想改 AWK 流程自己的檔案",
  "tool-policy/1#unreadable": "下了一個讀不懂的指令",
  // What the Worker itself reported.
  "agent-reported": "執行者回報這段內容要它做不該做的事",
  // Why a change the review approved waits for its owner before it is merged.
  "high-risk/1#ticket": "工單寫了風險高",
  "high-risk/1#release": "要合併到發版分支",
  "high-risk/1#migration": "改到資料庫的 migration",
  "high-risk/1#ci-build": "改到建置、測試、發版或容器的設定",
  "high-risk/1#dependency": "改到專案用到的套件或鎖定的版本",
  "high-risk/1#auth": "改到登入、權限或加密相關的程式",
  "high-risk/1#deletion": "刪掉很多檔案",
  "high-risk/1#project": "改到專案設為高風險的路徑",
};

/** isHighRisk says a verdict is a reason a change waits for its owner's approval, not something suspicious. */
export function isHighRisk(rule: string): boolean {
  return rule.startsWith("high-risk/");
}

// The lesson content policy says its reasons as sentences: they are the ids.
const LESSON_POLICY: Record<string, string> = {
  "it speaks of the workflow's permissions and checks": "講到流程的權限與檢查（例如跳過檢查）",
  "it reaches for another host": "要連到外面（網址或下載指令）",
  "it speaks of the workflow's own files and commands": "講到 AWK 流程自己的檔案或指令",
};

/** ruleWords says a verdict's rule in words, or the server's reason when the rule is not known here. */
export function ruleWords(rule: string, reason = ""): string {
  if (rule === "lesson-screen") return heldBackWords(reason);
  if (rule === "high-risk/1#deletion") {
    const m = /deletes (\d+) files, more than (\d+)/.exec(reason);
    if (m) return `刪掉 ${m[1]} 個檔案，超過 ${m[2]} 個`;
  }
  return RULES[rule] ?? (reason || rule);
}

const SOURCES: [RegExp, (m: RegExpExecArray) => string][] = [
  [/^lesson (L-\d+)$/, (m) => `教訓 ${m[1]}`],
  [/^lessons$/, () => "教訓"],
  [/^review comments on issue #(\d+)$/, (m) => `工單 #${m[1]} 的審查意見`],
  [/^file-level review comments$/, () => "PR 上的逐行意見"],
  [/^the Worker's change$/, () => "執行者的變更"],
  [/^the pull request$/, () => "這個 PR"],
  [/^issue #(\d+)$/, (m) => `工單 #${m[1]}`],
  [/^gh (pr|issue) (create|comment|review|edit)$/, (m) => `要送到 GitHub 的${m[1] === "pr" ? " PR " : "工單"}${{ create: "內容", comment: "留言", review: "審查", edit: "修改" }[m[2]]}`],
];

/** sourceWords says where a verdict's text came from, or the server's words when it is not known here. */
export function sourceWords(source: string): string {
  for (const [pattern, say] of SOURCES) {
    const m = pattern.exec(source.trim());
    if (m) return say(m);
  }
  return source;
}

/**
 * heldBackWords says why a lesson was held back. The server writes either a
 * content-policy sentence, or an injection rule's reason with the rule after
 * it: "tells the reader … (injection-rules/1#override-en)".
 */
export function heldBackWords(said: string): string {
  const text = said.trim();
  if (LESSON_POLICY[text]) return LESSON_POLICY[text];
  const rule = /\(([a-z-]+\/\d+#[a-z-]+)\)\s*$/.exec(text)?.[1];
  if (rule && RULES[rule]) return RULES[rule];
  return text;
}

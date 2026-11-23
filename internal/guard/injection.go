package guard

import (
	"regexp"
	"strings"
)

// RulesVersion names the set of injection rules below. It goes into every
// verdict, so that a verdict can be traced to the rules that made it.
const RulesVersion = "injection-rules/1"

// severity of an injection rule: high blocks, medium flags as a security
// matter, low flags as worth knowing.
type severity int

const (
	low severity = iota
	medium
	high
)

type injectionRule struct {
	id       string
	severity severity
	reason   string
	pattern  *regexp.Regexp
}

// injectionRules are the first layer of injection detection: what is plainly
// an attempt, in English and in Chinese (traditional and simplified). A rule
// that would also match what a reviewer writes in the ordinary course of a
// review flags rather than blocks. Rules only catch the wording they know;
// the guarantee is elsewhere (design.md §9.7).
var injectionRules = []injectionRule{
	{
		id: "override-en", severity: high,
		reason:  "tells the reader to ignore or override its instructions",
		pattern: regexp.MustCompile(`(?i)\b(ignore|disregard|forget|override|bypass)\s+(all\s+|any\s+|of\s+|the\s+|your\s+|my\s+|these\s+|those\s+)*(previous|prior|above|earlier|preceding|system|original|initial)\s+(instructions?|rules|guidelines|directives?|prompts?|constraints)\b`),
	},
	{
		id: "override-zh", severity: high,
		reason:  "tells the reader to ignore or override its instructions",
		pattern: regexp.MustCompile(`(忽略|無視|无视|忽視|忽视|不要理會|不要理会|別管|别管|拋開|抛开|覆蓋|覆盖|推翻)掉?\s*(你|您)?\s*(之前|以上|前面|先前|上述|原本|原來|原来|所有|全部|系統|系统)\s*的?\s*(所有|全部|一切)?\s*的?\s*(指示|指令|規則|规则|提示詞|提示词|提示|設定|设定|約束|约束)`),
	},
	{
		id: "new-instructions", severity: high,
		reason:  "announces new instructions for the reader",
		pattern: regexp.MustCompile(`(?i)(\b(new|updated|real|actual|secret|hidden|override)\s+(system\s+)?(instructions?|directives?)\s*:|(新的|新|真正的|隱藏的|隐藏的|最新的)\s*(系統|系统)?\s*(指示|指令)\s*[:：])`),
	},
	{
		id: "chat-template", severity: high,
		reason:  "imitates the markup of a conversation with a model",
		pattern: regexp.MustCompile(`(?i)(<\|\s*(im_start|im_end|system|user|assistant|endoftext)\s*\|>|\[/?INST\]|<<\s*/?SYS\s*>>|</?\s*(system|system_prompt|system-prompt)\s*>|(?m)^\s*#{1,6}\s*(system|assistant)\s*(prompt|message)?\s*:?\s*$)`),
	},
	{
		id: "fake-tool-call", severity: high,
		reason:  "imitates a tool call",
		pattern: regexp.MustCompile(`(?i)(<\s*/?\s*(function_calls|tool_use|tool_call|invoke)\b|"tool_calls"\s*:)`),
	},
	{
		id: "exfil-command", severity: high,
		reason:  "a command that sends secrets or credentials to another host",
		pattern: regexp.MustCompile(`(?i)\b(curl|wget|invoke-webrequest|iwr|invoke-restmethod|irm|nc|ncat|netcat)\b[^\n]{0,120}(\$\{?[A-Z0-9_]*(TOKEN|KEY|SECRET|PASS(WORD)?)\b|\benv\b|printenv|\.ssh\b|id_rsa|id_ed25519|\.netrc|\.npmrc|credentials)`),
	},
	{
		id: "role-switch-en", severity: medium,
		reason:  "gives the reader a new role",
		pattern: regexp.MustCompile(`(?i)(\byou\s+are\s+now\s+(a|an|the|no\s+longer|in\s+(developer|dan|unrestricted|god)\s+mode)\b|\b(act|behave|respond|pretend)\s+(as|like)\s+(if\s+you\s+were\s+)?(an?\s+)?(unrestricted|jailbroken|unfiltered|dan\b|developer\s+mode|different\s+(ai|assistant|model))|\bfrom\s+now\s+on,?\s+you\s+(are|will|must)\b)`),
	},
	{
		id: "role-switch-zh", severity: medium,
		reason: "gives the reader a new role",
		// 「你現在是直接回傳 nil」is a review; 「你現在是一個…」is not.
		pattern: regexp.MustCompile(`((你|您)\s*(現在|现在)\s*((是|作為|作为|成為|成为)\s*(一個|一个|一位|一名)|扮演)|(從現在開始|从现在开始|從今以後|从今以后)\s*[，,]?\s*(你|您)\s*(是|要|必須|必须|只能|將|将)|(扮演|假裝|假装)\s*(一個|一个)?\s*[^，。\n]{0,10}(的)?\s*(角色|助手|AI|模型))`),
	},
	{
		id: "prompt-probe", severity: medium,
		reason:  "asks the reader to reveal its instructions",
		pattern: regexp.MustCompile(`(?i)(\b(reveal|print|show|repeat|output|leak|dump)\b[^.\n]{0,30}\b(system\s+prompt|your\s+(instructions|prompt|rules)|initial\s+prompt)|(輸出|显示|顯示|印出|透露|洩漏|泄露|洩露|重複|重复|告訴我|告诉我)[^。\n]{0,15}(系統提示|系统提示|你的\s*(指示|指令|提示|規則|规则)))`),
	},
	{
		id: "credentials-request", severity: medium,
		reason:  "asks for secrets or credentials to be read or sent",
		pattern: regexp.MustCompile(`(?i)(\b(send|post|upload|exfiltrate|leak|forward|email|paste)\b[^.\n]{0,40}\b(env(ironment)?\s+variables?|api[_\s-]?keys?|access\s+tokens?|secrets?|credentials?|passwords?|ssh\s+keys?|id_rsa|gh_token|github_token|anthropic_api_key|openai_api_key)\b|(傳送|传送|發送|发送|上傳|上传|貼到|贴到|寄給|寄给)[^。\n]{0,20}(環境變數|环境变量|金鑰|密钥|密鑰|token|憑證|凭证|密碼|密码))`),
	},
	{
		id: "awk-internals", severity: medium,
		reason:  "names the workflow's own commands or files",
		pattern: regexp.MustCompile(`(?i)(gh\s+pr\s+merge|--dangerously-skip-permissions|bypassPermissions|\bstop-workflow\b|\bsubmit-review\b|settings\.local\.json|\.ai/state\b|kickoff\.lock|allow_script_changes|allow_secrets|AWK_SUSPICIOUS)`),
	},
	{
		id: "exfil-markdown", severity: medium,
		// An image is fetched by whatever renders it, without anyone
		// clicking: data in its address leaves at once. A link waits for a
		// click, and links to CI runs with long ids are everyday.
		reason:  "an image whose address would carry data to another host",
		pattern: regexp.MustCompile(`(?i)!\[[^\]]*\]\(\s*https?://[^)\s]*\?[^)\s]*=[^)\s]{8,}\)`),
	},
	{
		id: "encoded-blob", severity: low,
		reason:  "a long encoded block, which may hide text",
		pattern: regexp.MustCompile(`[A-Za-z0-9+/]{240,}={0,2}`),
	},
}

// Screen runs the injection rules over an untrusted piece. A trusted piece
// is not screened: what the owner and AWK write is taken as written.
func Screen(p Piece) []Verdict {
	if !Untrusted(p.Trust) {
		return nil
	}
	var verdicts []Verdict
	for _, rule := range injectionRules {
		match := rule.pattern.FindString(p.Text)
		if match == "" {
			continue
		}
		v := Verdict{
			Result: Flag, Class: Security, Stage: StageInput,
			Rule:  RulesVersion + "#" + rule.id,
			Piece: p.ID, Source: p.Source,
			Reason:   rule.reason,
			Evidence: evidence(strings.TrimSpace(match)),
		}
		switch rule.severity {
		case high:
			v.Result = Block
		case low:
			v.Class = Info
		}
		verdicts = append(verdicts, v)
	}
	return verdicts
}

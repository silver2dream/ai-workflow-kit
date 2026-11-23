package guard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The credentials are put together at run time: a test file that held them
// whole would itself be a file a secret scanner stops.
func fakeSecrets() map[string]string {
	alnum := func(n int) string { return strings.Repeat("aB3dE5", n/6+1)[:n] }
	upper := func(n int) string { return strings.Repeat("Q7ZK", n/4+1)[:n] }
	return map[string]string{
		"github-token":   "gh" + "p_" + alnum(36),
		"github-pat":     "github" + "_pat_" + alnum(82),
		"anthropic-key":  "sk-" + "ant-api03-" + alnum(40),
		"openai-key":     "sk-" + "proj-" + alnum(48),
		"aws-access-key": "AK" + "IA" + upper(16),
		"private-key":    "-----BEGIN RSA " + "PRIVATE KEY-----\nMIIE...",
		"slack-token":    "xo" + "xb-" + "1234567890-" + alnum(24),
		"google-api-key": "AI" + "za" + alnum(35),
		"jwt":            "ey" + "J" + alnum(20) + ".ey" + "J" + alnum(30) + "." + alnum(30),
	}
}

func TestScanOutbound_FindsEachKindOfCredential(t *testing.T) {
	for id, secret := range fakeSecrets() {
		text := "Done. I set the client up with " + secret + " so the tests can reach the service."
		verdicts := ScanOutbound(text, "gh pr comment")
		if !Blocks(verdicts) {
			t.Errorf("%s: not found in %q", id, text)
			continue
		}
		found := false
		for _, v := range verdicts {
			if v.Rule == OutboundVersion+"#"+id {
				found = true
			}
			if v.Stage != StageOutput || v.Class != Security || v.Source != "gh pr comment" {
				t.Errorf("%s: verdict %+v", id, v)
			}
			if strings.Contains(v.Evidence, secret) || strings.Contains(Summary(verdicts), secret) {
				t.Errorf("%s: the record holds the credential itself: %q", id, v.Evidence)
			}
		}
		if !found {
			t.Errorf("%s: found, but under %v", id, verdicts)
		}
		if len(verdicts) != 1 {
			t.Errorf("%s: one credential, %d verdicts: %v", id, len(verdicts), verdicts)
		}
	}
}

func TestScanOutbound_LeavesOrdinaryTextAlone(t *testing.T) {
	for _, text := range []string{
		"Set GITHUB_TOKEN and ANTHROPIC_API_KEY in the environment before the run.",
		"Tokens of GitHub start with ghp_ or gho_; a fine-grained one with github_pat_.",
		"The key comes from os.Getenv(\"OPENAI_API_KEY\"); never sk- literals in code.",
		"Commit 3f9a2c1d4e5b6a7c8d9e0f1a2b3c4d5e6f7a8b9c fixed it.",
		"sha256: 9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		"The public key is ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl",
		"-----BEGIN PUBLIC KEY----- is what the server sends.",
		"AKIA is how an AWS access key starts.",
		"修好了：設定檔讀取 token 的地方改成從環境變數拿。",
		"",
	} {
		if v := ScanOutbound(text, "test"); len(v) > 0 {
			t.Errorf("%q: %v", text, v)
		}
	}
}

func TestCheckGH_ReadsEveryTextACallCarries(t *testing.T) {
	secret := fakeSecrets()["github-token"]
	dir := t.TempDir()
	bodyFile := filepath.Join(dir, "body.md")
	if err := os.WriteFile(bodyFile, []byte("Summary\n\ntoken: "+secret+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cleanFile := filepath.Join(dir, "clean.md")
	if err := os.WriteFile(cleanFile, []byte("Summary\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	blocked := [][]string{
		{"pr", "comment", "12", "--body", "see " + secret},
		{"pr", "comment", "12", "--body=" + secret},
		{"issue", "comment", "3", "-b", secret},
		{"pr", "create", "--title", "fix " + secret, "--body", "ok"},
		{"issue", "create", "-t", secret, "-b", "ok"},
		{"issue", "create", "--title", "t", "--body-file", bodyFile},
		{"issue", "create", "--title", "t", "-F", bodyFile},
		{"pr", "review", "4", "--approve", "-b", secret},
		{"api", "--method", "POST", "repos/{owner}/{repo}/issues/1/comments", "-f", "body=" + secret},
		{"api", "repos/{owner}/{repo}/pulls/1/comments/2/replies", "--raw-field", "body=" + secret},
		{"api", "repos/{owner}/{repo}/issues/1/comments", "-F", "body=@" + bodyFile},
		{"api", "repos/{owner}/{repo}/issues/1/comments", "--input", bodyFile},
	}
	for _, args := range blocked {
		if v := CheckGH(args); !Blocks(v) {
			t.Errorf("gh %s: not stopped", strings.Join(args, " "))
		}
	}

	passed := [][]string{
		{"pr", "comment", "12", "--body", "All checks pass."},
		{"issue", "create", "--title", "t", "--body-file", cleanFile},
		{"issue", "edit", "3", "--add-label", "in-progress"},
		{"pr", "view", "12", "--json", "body"},
		{"api", "graphql", "-F", "owner={owner}", "-f", "query=query{viewer{login}}"},
		{"issue", "create", "--title", "t", "--body-file", filepath.Join(dir, "missing.md")},
	}
	for _, args := range passed {
		if v := CheckGH(args); len(v) > 0 {
			t.Errorf("gh %s: %v", strings.Join(args, " "), v)
		}
	}
}

func TestGHGate_RefusesAndRecords(t *testing.T) {
	root := t.TempDir()
	secret := fakeSecrets()["anthropic-key"]
	ctx := WithScope(context.Background(), Scope{StateRoot: root, Issue: 7})
	err := GHGate(ctx, []string{"pr", "comment", "7", "--body", "key " + secret})
	if err == nil || !strings.HasPrefix(err.Error(), "AWK guard:") || !errors.Is(err, ErrBlocked) {
		t.Fatalf("GHGate: %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("the error holds the credential: %v", err)
	}
	data, readErr := os.ReadFile(filepath.Join(root, ".ai", "state", "events", Session+".jsonl"))
	if readErr != nil {
		t.Fatalf("no verdict recorded: %v", readErr)
	}
	record := string(data)
	if !strings.Contains(record, OutboundVersion+"#anthropic-key") || !strings.Contains(record, `"stage":"output"`) || !strings.Contains(record, `"issue_id":7`) {
		t.Errorf("record: %s", record)
	}
	if strings.Contains(record, secret) {
		t.Errorf("the record holds the credential: %s", record)
	}

	if err := GHGate(ctx, []string{"pr", "comment", "7", "--body", "fine"}); err != nil {
		t.Errorf("a clean call: %v", err)
	}
}

func TestWithScope_KeepsWhatItIsNotGiven(t *testing.T) {
	ctx := WithScope(context.Background(), Scope{StateRoot: "/p", Issue: 3})
	if got := ScopeOf(WithScope(ctx, Scope{StateRoot: "/q"})); got != (Scope{StateRoot: "/q", Issue: 3}) {
		t.Errorf("scope: %+v", got)
	}
	if got := ScopeOf(context.Background()); got != (Scope{}) {
		t.Errorf("no scope: %+v", got)
	}
}

// luhnDigit is the check digit a card number's other digits call for.
func luhnDigit(digits string) byte {
	sum := 0
	for i := 0; i < len(digits); i++ {
		d := int(digits[len(digits)-1-i] - '0')
		if i%2 == 0 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return byte('0' + (10-sum%10)%10)
}

// twDigit is the check digit a national ID's letter and eight digits call for.
func twDigit(id string) byte {
	code := twLetters[id[0]]
	sum := code/10 + (code%10)*9
	for i := 1; i <= 8; i++ {
		sum += int(id[i]-'0') * (9 - i)
	}
	return byte('0' + (10-sum%10)%10)
}

// wrong is a check digit that is not the right one.
func wrong(d byte) string { return string('0' + (d-'0'+1)%10) }

func spaced(n string, sep string) string {
	var parts []string
	for len(n) > 4 {
		parts = append(parts, n[:4])
		n = n[4:]
	}
	return strings.Join(append(parts, n), sep)
}

func TestScanOutbound_PersonalData(t *testing.T) {
	// The numbers are completed here, not written down: a number written
	// down with a right check digit could be someone's.
	visa, mc, amex := "453914880343646", "542523343010990", "37424545540012"
	idF, idB := "F13123221", "B22034951"
	for _, text := range []string{
		"refund to card " + spaced(visa+string(luhnDigit(visa)), " ") + " please",
		"card: " + spaced(mc+string(luhnDigit(mc)), "-"),
		"amex " + amex + string(luhnDigit(amex)),
		"客戶身分證 " + idF + string(twDigit(idF)) + " 的訂單",
		"id=" + idB + string(twDigit(idB)),
	} {
		v := ScanOutbound(text, "test")
		if !Blocks(v) {
			t.Errorf("%q: not found", text)
			continue
		}
		if !strings.Contains(v[0].Reason, "personal data") {
			t.Errorf("%q: %s", text, v[0].Reason)
		}
		for _, verdict := range v {
			if !strings.HasSuffix(verdict.Evidence, "[REDACTED]") || len([]rune(verdict.Evidence)) > 15 {
				t.Errorf("%q: evidence %q keeps the number", text, verdict.Evidence)
			}
		}
	}
	// Check digits wrong, a provider's test card, the example ID, numbers
	// that are no card.
	for _, text := range []string{
		"refund to card " + spaced(visa+wrong(luhnDigit(visa)), " "),
		"use the test card 4242 4242 4242 4242 in staging",
		"Stripe's 4000056655665556 is a debit test card",
		"身分證欄位的範例是 A123456789",
		"id=" + idF + wrong(twDigit(idF)),
		"snowflake id 1732345678901234567",
		"timestamp 1732345678901",
		"phone +886 912 345 678",
	} {
		if v := ScanOutbound(text, "test"); len(v) > 0 {
			t.Errorf("%q: %v", text, v)
		}
	}
}

func TestReveal(t *testing.T) {
	text := "push​to‮main"
	if got := Reveal(text); got != "push<U+200B>to<U+202E>main" {
		t.Errorf("Reveal = %q", got)
	}
	if got := Reveal("plain 中文"); got != "plain 中文" {
		t.Errorf("plain text changed: %q", got)
	}
}

func TestRecordDecision(t *testing.T) {
	root := t.TempDir()
	RecordDecision(root, Decision{Stage: StageMemory, Rule: "lesson-screen", Piece: "L-3", Said: "why", Decision: ApproveDecision, By: "cli"})
	data, err := os.ReadFile(filepath.Join(root, ".ai", "state", "events", Session+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type":"guardrail_decision"`, `"component":"guard"`, `"piece":"L-3"`, `"decision":"approve"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("record lacks %s: %s", want, data)
		}
	}
	RecordDecision("", Decision{Piece: "nowhere"}) // no project, no stream: nothing, and no panic
}

func TestRedact(t *testing.T) {
	gh := fakeSecrets()["github-token"]
	ant := fakeSecrets()["anthropic-key"]
	text := "use " + gh + " and " + ant + " then " + gh + "; card 4242 4242 4242 4242 is a test card"
	got, kinds := Redact(text)
	want := "use [REDACTED:github-token] and [REDACTED:anthropic-key] then [REDACTED:github-token]; card 4242 4242 4242 4242 is a test card"
	if got != want {
		t.Errorf("Redact:\n%s\nwant\n%s", got, want)
	}
	if strings.Join(kinds, ",") != "github-token,anthropic-key" {
		t.Errorf("kinds = %v (an Anthropic key is no OpenAI key too)", kinds)
	}
	if got, kinds := Redact("nothing to hide"); got != "nothing to hide" || kinds != nil {
		t.Errorf("plain: %q %v", got, kinds)
	}
}

package guard

import (
	"strings"
	"testing"
)

func ruleIDs(reasons []RiskReason) string {
	var ids []string
	for _, r := range reasons {
		ids = append(ids, strings.TrimPrefix(r.Rule, RiskVersion+"#"))
	}
	return strings.Join(ids, " ")
}

// Each rule, and what it is not.
func TestHighRisk(t *testing.T) {
	for _, c := range []struct {
		name string
		in   RiskInput
		want string
	}{
		{"ordinary work", RiskInput{Files: []string{"rally/score.go", "rally/score_test.go", "docs/rules.md"}, TicketRisk: "low"}, ""},
		{"a ticket of medium risk", RiskInput{Files: []string{"a.go"}, TicketRisk: "medium"}, ""},
		{"a ticket of high risk", RiskInput{Files: []string{"a.go"}, TicketRisk: "High"}, "ticket"},
		{"a release", RiskInput{Files: []string{"a.go"}, Release: true}, "release"},
		{"migrations", RiskInput{Files: []string{"db/migrations/0003_users.sql", "backend/migration/x.go", "alembic/versions/ab12.py", "db/migrate/20261128_add.rb", "sql/V3_1__add_users.sql"}}, "migration"},
		{"not migrations", RiskInput{Files: []string{"internal/migrate/migrate.go", "cmd/migrate.go", "docs/migrations.md"}}, ""},
		{"CI and the build", RiskInput{Files: []string{".github/workflows/ci.yml", "Makefile", "deploy/Dockerfile", "docker-compose.yaml"}}, "ci-build"},
		{"dependencies and their locks", RiskInput{Files: []string{"go.mod", "go.sum", "web/package.json", "web/package-lock.json", "yarn.lock", "requirements-dev.txt", "Cargo.lock"}}, "dependency"},
		{"authentication and cryptography", RiskInput{Files: []string{"internal/auth/token.go", "pkg/OAuth2/client.go", "web/src/login-form.tsx", "keys/jwt_sign.go", "internal/crypto/aes.go", "rbac.yaml"}}, "auth"},
		{"words that are common", RiskInput{Files: []string{"internal/session/manager.go", "lexer/tokens.go", "internal/worker/security.go", "keyboard/keys.go", "author.go"}}, ""},
		{"many files deleted", RiskInput{Files: []string{"a.go"}, Deleted: 21}, "deletion"},
		{"the default threshold", RiskInput{Files: []string{"a.go"}, Deleted: 20}, ""},
		{"a threshold of the project", RiskInput{Files: []string{"a.go"}, Deleted: 6, DeleteThreshold: 5}, "deletion"},
		{"the project's paths", RiskInput{Files: []string{"Billing/Invoice.go", "rally/a.go"}, Paths: []string{"billing/**", " "}}, "project"},
		{"several at once", RiskInput{Files: []string{"go.mod", "internal/auth/a.go"}, TicketRisk: "high", Release: true}, "ticket release dependency auth"},
	} {
		if got := ruleIDs(HighRisk(c.in)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	// The files a rule found, sorted; the deletion says its numbers.
	r := HighRisk(RiskInput{Files: []string{"web/yarn.lock", "go.mod"}, Deleted: 30})
	if len(r) != 2 || strings.Join(r[0].Files, ",") != "go.mod,web/yarn.lock" || r[1].Reason != "it deletes 30 files, more than 20" {
		t.Errorf("reasons: %+v", r)
	}
}

// A reason is kept as a flag, with the files it found, five at most.
func TestRiskReason_Verdict(t *testing.T) {
	v := RiskReason{Rule: RiskVersion + "#dependency", Reason: "it changes what the project depends on", Files: []string{"a", "b", "c", "d", "e", "f", "g"}}.Verdict()
	if v.Result != Flag || v.Class != Info || v.Stage != StageOutput || v.Rule != "high-risk/1#dependency" || v.Evidence != "a, b, c, d, e and 2 more" {
		t.Errorf("verdict: %+v", v)
	}
}

// The files of a pull request's diff, a rename's both names, and the files
// it deletes.
func TestChangeOf(t *testing.T) {
	diff := "diff --git a/rally/score.go b/rally/score.go\n" +
		"index 1..2 100644\n--- a/rally/score.go\n+++ b/rally/score.go\n@@ -1 +1 @@\n-a\n+b\n" +
		"diff --git a/old.go b/old.go\ndeleted file mode 100644\nindex 1..0\n--- a/old.go\n+++ /dev/null\n" +
		"diff --git a/gone.txt b/gone.txt\r\ndeleted file mode 100644\r\n" +
		"diff --git a/web/a.ts b/web/b.ts\nsimilarity index 90%\nrename from web/a.ts\nrename to web/b.ts\n" +
		"diff --git a/new.go b/new.go\nnew file mode 100644\n"
	files, deleted := ChangeOf(diff)
	if got := strings.Join(files, " "); got != "rally/score.go old.go gone.txt web/b.ts web/a.ts new.go" || deleted != 2 {
		t.Errorf("files %q, deleted %d", got, deleted)
	}
	if files, deleted := ChangeOf(""); len(files) != 0 || deleted != 0 {
		t.Errorf("no diff: %v %d", files, deleted)
	}
}

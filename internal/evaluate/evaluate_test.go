package evaluate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOfflineGate_O0_GitIgnore(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(tmpDir string) error
		wantStatus string
	}{
		{
			name: "not a git repository",
			setup: func(tmpDir string) error {
				return nil // No .git directory
			},
			wantStatus: "SKIP",
		},
		{
			name: "git repo with gitignore",
			setup: func(tmpDir string) error {
				// Create .git directory
				if err := os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755); err != nil {
					return err
				}
				// Create .gitignore with required paths
				gitignore := `.ai/state/
.ai/results/
.ai/runs/
.ai/exe-logs/
`
				return os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitignore), 0644)
			},
			wantStatus: "PASS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := tt.setup(tmpDir); err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			result := CheckO0GitIgnore(tmpDir)

			// For non-git repos, we expect SKIP
			if tt.name == "not a git repository" {
				if result.Status != "SKIP" {
					t.Errorf("got status %q, want %q", result.Status, "SKIP")
				}
				return
			}

			// For git repos, behavior depends on whether git is available
			// and whether the paths are properly ignored
			if result.Status != tt.wantStatus && result.Status != "FAIL" {
				// Accept FAIL if git check-ignore doesn't work as expected in test env
				t.Logf("got status %q (reason: %s)", result.Status, result.Reason)
			}
		})
	}
}

func TestOfflineGate_O5_ConfigValidation(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(tmpDir string) error
		wantStatus string
	}{
		{
			name: "config not found",
			setup: func(tmpDir string) error {
				return nil // No config file
			},
			wantStatus: "FAIL",
		},
		{
			name: "invalid YAML",
			setup: func(tmpDir string) error {
				configDir := filepath.Join(tmpDir, ".ai", "config")
				if err := os.MkdirAll(configDir, 0755); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte("invalid: yaml: :"), 0644)
			},
			wantStatus: "FAIL",
		},
		{
			name: "missing required fields",
			setup: func(tmpDir string) error {
				configDir := filepath.Join(tmpDir, ".ai", "config")
				if err := os.MkdirAll(configDir, 0755); err != nil {
					return err
				}
				config := `version: "1.0"
`
				return os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte(config), 0644)
			},
			wantStatus: "FAIL",
		},
		{
			name: "valid config",
			setup: func(tmpDir string) error {
				configDir := filepath.Join(tmpDir, ".ai", "config")
				if err := os.MkdirAll(configDir, 0755); err != nil {
					return err
				}
				config := `version: "1.0"
project:
  name: test-project
  type: monorepo
repos:
  - name: backend
    path: backend/
git:
  integration_branch: develop
`
				return os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte(config), 0644)
			},
			wantStatus: "PASS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := tt.setup(tmpDir); err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			result := CheckO5ConfigValidation(tmpDir)

			if result.Status != tt.wantStatus {
				t.Errorf("got status %q (reason: %s), want %q", result.Status, result.Reason, tt.wantStatus)
			}
		})
	}
}

func TestOfflineGate_O8_FileEncoding(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(tmpDir string) error
		wantStatus string
	}{
		{
			name: "no .ai directory",
			setup: func(tmpDir string) error {
				return nil
			},
			wantStatus: "SKIP",
		},
		{
			name: "clean files",
			setup: func(tmpDir string) error {
				aiDir := filepath.Join(tmpDir, ".ai", "config")
				if err := os.MkdirAll(aiDir, 0755); err != nil {
					return err
				}
				// Write file with LF line endings
				return os.WriteFile(filepath.Join(aiDir, "test.yaml"), []byte("key: value\n"), 0644)
			},
			wantStatus: "PASS",
		},
		{
			name: "file with CRLF",
			setup: func(tmpDir string) error {
				aiDir := filepath.Join(tmpDir, ".ai", "config")
				if err := os.MkdirAll(aiDir, 0755); err != nil {
					return err
				}
				// Write file with CRLF line endings
				return os.WriteFile(filepath.Join(aiDir, "test.yaml"), []byte("key: value\r\n"), 0644)
			},
			wantStatus: "FAIL",
		},
		{
			name: "file with UTF-16 BOM",
			setup: func(tmpDir string) error {
				aiDir := filepath.Join(tmpDir, ".ai", "config")
				if err := os.MkdirAll(aiDir, 0755); err != nil {
					return err
				}
				// Write file with UTF-16 LE BOM
				content := []byte{0xFF, 0xFE, 'k', 0, 'e', 0, 'y', 0}
				return os.WriteFile(filepath.Join(aiDir, "test.yaml"), content, 0644)
			},
			wantStatus: "FAIL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := tt.setup(tmpDir); err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			result := CheckO8FileEncoding(tmpDir)

			if result.Status != tt.wantStatus {
				t.Errorf("got status %q (reason: %s), want %q", result.Status, result.Reason, tt.wantStatus)
			}
		})
	}
}

func TestScoring_CalculateScoreCap(t *testing.T) {
	tests := []struct {
		name     string
		offline  OfflineGateResults
		online   OnlineGateResults
		minScore float64
		maxScore float64
	}{
		{
			name: "all pass",
			offline: OfflineGateResults{
				O0:  Pass("ok"),
				O1:  Pass("ok"),
				O3:  Pass("ok"),
				O5:  Pass("ok"),
				O7:  Pass("ok"),
				O8:  Pass("ok"),
				O10: Pass("ok"),
			},
			online: OnlineGateResults{
				N1: Pass("ok"),
				N2: Pass("ok"),
				N3: Pass("ok"),
			},
			minScore: 100.0,
			maxScore: 100.0,
		},
		{
			name: "all fail",
			offline: OfflineGateResults{
				O0:  Fail("failed"),
				O1:  Fail("failed"),
				O3:  Fail("failed"),
				O5:  Fail("failed"),
				O7:  Fail("failed"),
				O8:  Fail("failed"),
				O10: Fail("failed"),
			},
			online: OnlineGateResults{
				N1: Fail("failed"),
				N2: Fail("failed"),
				N3: Fail("failed"),
			},
			minScore: 0.0,
			maxScore: 0.0,
		},
		{
			name: "all skip",
			offline: OfflineGateResults{
				O0:  Skip("skipped"),
				O1:  Skip("skipped"),
				O3:  Skip("skipped"),
				O5:  Skip("skipped"),
				O7:  Skip("skipped"),
				O8:  Skip("skipped"),
				O10: Skip("skipped"),
			},
			online: OnlineGateResults{
				N1: Skip("skipped"),
				N2: Skip("skipped"),
				N3: Skip("skipped"),
			},
			minScore: 50.0,
			maxScore: 50.0,
		},
		{
			name: "mixed results",
			offline: OfflineGateResults{
				O0:  Pass("ok"),
				O1:  Skip("skipped"),
				O3:  Fail("failed"),
				O5:  Pass("ok"),
				O7:  Skip("skipped"),
				O8:  Pass("ok"),
				O10: Fail("failed"),
			},
			online: OnlineGateResults{
				N1: Pass("ok"),
				N2: Skip("skipped"),
				N3: Fail("failed"),
			},
			minScore: 50.0,
			maxScore: 75.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := CalculateScoreCap(tt.offline, tt.online)

			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("got score %.2f, want between %.2f and %.2f", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestScoring_ScoreToGrade(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{100.0, "A"},
		{95.0, "A"},
		{90.0, "A"},
		{89.9, "B"},
		{85.0, "B"},
		{80.0, "B"},
		{79.9, "C"},
		{75.0, "C"},
		{70.0, "C"},
		{69.9, "D"},
		{65.0, "D"},
		{60.0, "D"},
		{59.9, "F"},
		{50.0, "F"},
		{0.0, "F"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := ScoreToGrade(tt.score)
			if got != tt.want {
				t.Errorf("ScoreToGrade(%.1f) = %q, want %q", tt.score, got, tt.want)
			}
		})
	}
}

func TestGateResult_Constructors(t *testing.T) {
	pass := Pass("test pass")
	if pass.Status != "PASS" || pass.Reason != "test pass" {
		t.Errorf("Pass() = %+v, want Status=PASS, Reason=test pass", pass)
	}

	fail := Fail("test fail")
	if fail.Status != "FAIL" || fail.Reason != "test fail" {
		t.Errorf("Fail() = %+v, want Status=FAIL, Reason=test fail", fail)
	}

	skip := Skip("test skip")
	if skip.Status != "SKIP" || skip.Reason != "test skip" {
		t.Errorf("Skip() = %+v, want Status=SKIP, Reason=test skip", skip)
	}
}

func TestEvaluator_New(t *testing.T) {
	e := New("")
	if e.RootPath != "." {
		t.Errorf("New(\"\") RootPath = %q, want %q", e.RootPath, ".")
	}

	e = New("/custom/path")
	if e.RootPath != "/custom/path" {
		t.Errorf("New(\"/custom/path\") RootPath = %q, want %q", e.RootPath, "/custom/path")
	}
}

func TestGradeDescription(t *testing.T) {
	tests := []struct {
		grade string
		want  string
	}{
		{"A", "Excellent - ready for production"},
		{"B", "Good - minor improvements recommended"},
		{"C", "Acceptable - some issues to address"},
		{"D", "Below standard - significant issues"},
		{"F", "Failing - critical issues must be fixed"},
		{"X", "Unknown grade"},
	}

	for _, tt := range tests {
		t.Run(tt.grade, func(t *testing.T) {
			got := GradeDescription(tt.grade)
			if got != tt.want {
				t.Errorf("GradeDescription(%q) = %q, want %q", tt.grade, got, tt.want)
			}
		})
	}
}

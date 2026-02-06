package repo

import (
	"testing"
)

func TestResolver_FindByWorktreePath(t *testing.T) {
	repos := []Config{
		{Name: "backend", Path: "backend", Language: "go", Type: "directory"},
		{Name: "frontend", Path: "frontend", Language: "typescript", Type: "directory"},
	}
	resolver := NewResolver(repos)

	tests := []struct {
		name         string
		worktreePath string
		wantRepo     string
		wantLang     string
	}{
		{
			name:         "matches frontend by path segment",
			worktreePath: "/project/.worktrees/issue-1/frontend",
			wantRepo:     "frontend",
			wantLang:     "typescript",
		},
		{
			name:         "matches backend by path segment",
			worktreePath: "/project/.worktrees/issue-1/backend",
			wantRepo:     "backend",
			wantLang:     "go",
		},
		{
			name:         "path with trailing slash",
			worktreePath: "/project/.worktrees/issue-1/frontend/",
			wantRepo:     "frontend",
			wantLang:     "typescript",
		},
		{
			name:         "windows style path",
			worktreePath: "D:\\projects\\.worktrees\\issue-1\\frontend",
			wantRepo:     "frontend",
			wantLang:     "typescript",
		},
		{
			name:         "no false positive - backend-old should not match backend",
			worktreePath: "/project/.worktrees/issue-1/backend-old",
			wantRepo:     "",
			wantLang:     "",
		},
		{
			name:         "no false positive - mybackend should not match backend",
			worktreePath: "/project/.worktrees/issue-1/mybackend",
			wantRepo:     "",
			wantLang:     "",
		},
		{
			name:         "empty path returns nil",
			worktreePath: "",
			wantRepo:     "",
			wantLang:     "",
		},
		{
			name:         "NOT_FOUND returns nil",
			worktreePath: "NOT_FOUND",
			wantRepo:     "",
			wantLang:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolver.FindByWorktreePath(tt.worktreePath)
			if tt.wantRepo == "" {
				if got != nil {
					t.Errorf("expected no match, got %s", got.Name)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected match for %s, got nil", tt.wantRepo)
			}
			if got.Name != tt.wantRepo {
				t.Errorf("Name = %s, want %s", got.Name, tt.wantRepo)
			}
			if got.Language != tt.wantLang {
				t.Errorf("Language = %s, want %s", got.Language, tt.wantLang)
			}
		})
	}
}

func TestResolver_FindByWorktreePath_WithRootRepo(t *testing.T) {
	repos := []Config{
		{Name: "main", Path: "./", Language: "go", Type: "root"},
	}
	resolver := NewResolver(repos)

	// Root repo should match any path that doesn't match a specific directory
	got := resolver.FindByWorktreePath("/project/.worktrees/issue-1")
	if got == nil {
		t.Fatal("expected root repo to match, got nil")
	}
	if got.Name != "main" {
		t.Errorf("Name = %s, want main", got.Name)
	}
}

func TestResolver_FindByWorktreePath_DirectoryTakesPrecedence(t *testing.T) {
	repos := []Config{
		{Name: "root", Path: "./", Language: "go", Type: "root"},
		{Name: "frontend", Path: "frontend", Language: "typescript", Type: "directory"},
	}
	resolver := NewResolver(repos)

	// Directory repo should take precedence over root
	got := resolver.FindByWorktreePath("/project/.worktrees/issue-1/frontend")
	if got == nil {
		t.Fatal("expected frontend to match, got nil")
	}
	if got.Name != "frontend" {
		t.Errorf("Name = %s, want frontend", got.Name)
	}
}

func TestResolver_FindByName(t *testing.T) {
	repos := []Config{
		{Name: "Backend", Path: "backend", Language: "go"},
		{Name: "Frontend", Path: "frontend", Language: "typescript"},
	}
	resolver := NewResolver(repos)

	tests := []struct {
		name     string
		repoName string
		want     string
	}{
		{"exact match", "Backend", "Backend"},
		{"case insensitive", "backend", "Backend"},
		{"case insensitive upper", "FRONTEND", "Frontend"},
		{"not found", "unknown", ""},
		{"with spaces", " frontend ", "Frontend"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolver.FindByName(tt.repoName)
			if tt.want == "" {
				if got != nil {
					t.Errorf("expected nil, got %s", got.Name)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected %s, got nil", tt.want)
			}
			if got.Name != tt.want {
				t.Errorf("Name = %s, want %s", got.Name, tt.want)
			}
		})
	}
}

func TestContainsPathSegment(t *testing.T) {
	tests := []struct {
		name     string
		fullPath string
		segment  string
		want     bool
	}{
		{"exact segment match", "/a/b/frontend/c", "frontend", true},
		{"multi-segment match", "/a/b/src/frontend", "src/frontend", true},
		{"no partial match", "/a/b/frontend-old/c", "frontend", false},
		{"no partial match prefix", "/a/b/myfrontend/c", "frontend", false},
		{"empty segment", "/a/b/c", "", false},
		{"segment not in path", "/a/b/c", "frontend", false},
		{"segment at start", "frontend/src/app", "frontend", true},
		{"segment at end", "/a/b/frontend", "frontend", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsPathSegment(tt.fullPath, tt.segment)
			if got != tt.want {
				t.Errorf("containsPathSegment(%q, %q) = %v, want %v",
					tt.fullPath, tt.segment, got, tt.want)
			}
		})
	}
}

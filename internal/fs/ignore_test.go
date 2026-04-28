package fs

import "testing"

func TestIsConflictPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "history.txt", want: false},
		{path: "history.txt.roodox-conflict-20260322-120102.123456789-ab12cd", want: true},
		{path: "dir/main.cpp.roodox-conflict-20260322-120102-ab12cd", want: true},
	}

	for _, tt := range tests {
		if got := IsConflictPath(tt.path); got != tt.want {
			t.Fatalf("IsConflictPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestShouldIgnoreInProjectScan(t *testing.T) {
	if !ShouldIgnoreInProjectScan(".git") {
		t.Fatal("expected .git to be ignored in project scan")
	}
	if !ShouldIgnoreInProjectScan("build.log.roodox-conflict-20260322-120102.123456789-ab12cd") {
		t.Fatal("expected conflict copy to be ignored in project scan")
	}
	if ShouldIgnoreInProjectScan("src/main.cpp") {
		t.Fatal("unexpected ignore for normal source file")
	}
}

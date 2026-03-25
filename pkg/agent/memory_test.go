package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewMemoryStore_CreatesDirectory(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "ws")
	ms := NewMemoryStore(workspace)

	memDir := filepath.Join(workspace, "memory")
	info, err := os.Stat(memDir)
	if err != nil {
		t.Fatalf("memory directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected memory path to be a directory")
	}
	if ms.workspace != workspace {
		t.Errorf("workspace = %q, want %q", ms.workspace, workspace)
	}
}

func TestReadLongTerm_Empty(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	got := ms.ReadLongTerm()
	if got != "" {
		t.Errorf("ReadLongTerm on fresh store = %q, want empty", got)
	}
}

func TestWriteAndReadLongTerm(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())

	content := "# Memory\n\nImportant fact."
	if err := ms.WriteLongTerm(content); err != nil {
		t.Fatalf("WriteLongTerm failed: %v", err)
	}
	got := ms.ReadLongTerm()
	if got != content {
		t.Errorf("ReadLongTerm = %q, want %q", got, content)
	}
}

func TestWriteLongTerm_Overwrites(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())

	ms.WriteLongTerm("first")
	ms.WriteLongTerm("second")

	got := ms.ReadLongTerm()
	if got != "second" {
		t.Errorf("ReadLongTerm after overwrite = %q, want %q", got, "second")
	}
}

func TestReadToday_Empty(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	got := ms.ReadToday()
	if got != "" {
		t.Errorf("ReadToday on fresh store = %q, want empty", got)
	}
}

func TestAppendToday_CreatesWithHeader(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())

	if err := ms.AppendToday("Some note"); err != nil {
		t.Fatalf("AppendToday failed: %v", err)
	}

	got := ms.ReadToday()
	today := time.Now().Format("2006-01-02")
	if !strings.HasPrefix(got, "# "+today) {
		t.Errorf("expected date header, got %q", got)
	}
	if !strings.Contains(got, "Some note") {
		t.Errorf("expected note content in %q", got)
	}
}

func TestAppendToday_AppendsToExisting(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())

	ms.AppendToday("First note")
	ms.AppendToday("Second note")

	got := ms.ReadToday()
	if !strings.Contains(got, "First note") {
		t.Errorf("expected first note in %q", got)
	}
	if !strings.Contains(got, "Second note") {
		t.Errorf("expected second note in %q", got)
	}
}

func TestGetTodayFile_Format(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	todayFile := ms.getTodayFile()

	today := time.Now().Format("20060102")
	monthDir := today[:6]

	if !strings.Contains(todayFile, monthDir) {
		t.Errorf("expected month dir %q in path %q", monthDir, todayFile)
	}
	if !strings.HasSuffix(todayFile, today+".md") {
		t.Errorf("expected file to end with %s.md, got %q", today, todayFile)
	}
}

func TestGetRecentDailyNotes_Empty(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	got := ms.GetRecentDailyNotes(3)
	if got != "" {
		t.Errorf("GetRecentDailyNotes on fresh store = %q, want empty", got)
	}
}

func TestGetRecentDailyNotes_IncludesToday(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	ms.AppendToday("Today's note")

	got := ms.GetRecentDailyNotes(1)
	if !strings.Contains(got, "Today's note") {
		t.Errorf("expected today's note in %q", got)
	}
}

func TestGetMemoryContext_Empty(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	got := ms.GetMemoryContext()
	if got != "" {
		t.Errorf("GetMemoryContext on fresh store = %q, want empty", got)
	}
}

func TestGetMemoryContext_LongTermOnly(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	ms.WriteLongTerm("Important")

	got := ms.GetMemoryContext()
	if !strings.Contains(got, "## Long-term Memory") {
		t.Errorf("expected Long-term Memory header in %q", got)
	}
	if !strings.Contains(got, "Important") {
		t.Errorf("expected content in %q", got)
	}
	if strings.Contains(got, "## Recent Daily Notes") {
		t.Error("should not have Recent Daily Notes section")
	}
}

func TestGetMemoryContext_DailyNotesOnly(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	ms.AppendToday("Note")

	got := ms.GetMemoryContext()
	if !strings.Contains(got, "## Recent Daily Notes") {
		t.Errorf("expected Recent Daily Notes header in %q", got)
	}
	if strings.Contains(got, "## Long-term Memory") {
		t.Error("should not have Long-term Memory section")
	}
}

func TestGetMemoryContext_Both(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	ms.WriteLongTerm("Memory")
	ms.AppendToday("Note")

	got := ms.GetMemoryContext()
	if !strings.Contains(got, "## Long-term Memory") {
		t.Error("missing Long-term Memory header")
	}
	if !strings.Contains(got, "## Recent Daily Notes") {
		t.Error("missing Recent Daily Notes header")
	}
	if !strings.Contains(got, "---") {
		t.Error("missing separator between sections")
	}
}

func TestWriteLongTerm_Permissions(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	ms.WriteLongTerm("secret")

	info, err := os.Stat(ms.memoryFile)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	perm := info.Mode().Perm()
	if perm&0o077 != 0 {
		t.Errorf("memory file should be owner-only, got %o", perm)
	}
}

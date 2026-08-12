package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The rotation branch only runs when the date changes, which would otherwise go
// untested until a server happened to stay up past midnight.
func TestDailyFileRotatesOnDateChange(t *testing.T) {
	dir := t.TempDir()
	d := &dailyFile{dir: dir}
	defer d.Close()

	if _, err := d.Write([]byte("first\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if d.day != today {
		t.Fatalf("day = %q, want %q", d.day, today)
	}

	// Pretend the process has been running since yesterday.
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	oldPath := filepath.Join(dir, "app-"+yesterday+".log")
	d.Close()
	f, err := os.Create(oldPath)
	if err != nil {
		t.Fatalf("create old file: %v", err)
	}
	d.f, d.day = f, yesterday

	if _, err := d.Write([]byte("second\n")); err != nil {
		t.Fatalf("write after rollover: %v", err)
	}

	// The second write must land in today's file, not yesterday's.
	got, err := os.ReadFile(filepath.Join(dir, "app-"+today+".log"))
	if err != nil {
		t.Fatalf("read today's file: %v", err)
	}
	if string(got) != "first\nsecond\n" {
		t.Errorf("today's file = %q, want %q", got, "first\nsecond\n")
	}
	if old, err := os.ReadFile(oldPath); err != nil || len(old) != 0 {
		t.Errorf("yesterday's file = %q (err %v), want it left empty", old, err)
	}
}

func TestInitCreatesLogDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "logs")
	c, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer c.Close()

	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("log dir not created: %v", err)
	}
}

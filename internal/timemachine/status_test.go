package timemachine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseStatusIdle(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "status-idle.plist"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := ParseStatus(data)
	if err != nil {
		t.Fatal(err)
	}
	if st.Running {
		t.Fatal("idle should not be running")
	}
	if st.Percent != -1 {
		t.Fatalf("percent = %v", st.Percent)
	}
}

func TestParseStatusCopying(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "status-copying.plist"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := ParseStatus(data)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running {
		t.Fatal("expected running")
	}
	if st.Phase != "Copying" {
		t.Fatalf("phase = %q", st.Phase)
	}
	if st.Files != 548346 {
		t.Fatalf("files = %d", st.Files)
	}
	if st.TotalFiles != 1048225 {
		t.Fatalf("totalFiles = %d", st.TotalFiles)
	}
	if st.Bytes != 919235073 {
		t.Fatalf("bytes = %d", st.Bytes)
	}
	if st.RawPercent < 0.019 || st.RawPercent > 0.02 {
		t.Fatalf("raw percent = %v", st.RawPercent)
	}
	if st.TimeRemaining != 143771 {
		t.Fatalf("time remaining = %v", st.TimeRemaining)
	}
	if st.DestinationMountPoint != "/Volumes/tm2" {
		t.Fatalf("dest = %q", st.DestinationMountPoint)
	}
}

func TestParseStatusPercentInProgress(t *testing.T) {
	const xml = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>BackupPhase</key>
	<string>Copying</string>
	<key>Running</key>
	<true/>
	<key>Progress</key>
	<dict>
		<key>Percent</key>
		<real>0.3645306849337138</real>
		<key>_raw_Percent</key>
		<real>0.3645306849337138</real>
		<key>files</key>
		<integer>33325</integer>
		<key>totalFiles</key>
		<integer>3462898</integer>
	</dict>
</dict>
</plist>`
	st, err := ParseStatus([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	if st.Percent < 0.36 || st.Percent > 0.37 {
		t.Fatalf("percent = %v", st.Percent)
	}
	if st.TotalFiles != 3462898 {
		t.Fatalf("totalFiles = %d", st.TotalFiles)
	}
}

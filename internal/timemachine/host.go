package timemachine

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// Host is the Time Machine / diskutil surface. Tests fake this; production
// never sends SIGINT to startbackup (Time Machine keeps no partial progress).
type Host interface {
	DiskPresent(uuid string) bool
	VolumeMounted(name string) bool
	Mount(uuid string) error
	Unmount(uuid string) error
	Status() (Status, error)
	StartBackup() (stdout string, err error)
}

type ExecHost struct {
	Diskutil string
	TMUtil   string
	MountBin string
}

func (h ExecHost) diskutil() string {
	if h.Diskutil != "" {
		return h.Diskutil
	}
	return "diskutil"
}

func (h ExecHost) tmutil() string {
	if h.TMUtil != "" {
		return h.TMUtil
	}
	return "tmutil"
}

func (h ExecHost) mountBin() string {
	if h.MountBin != "" {
		return h.MountBin
	}
	return "mount"
}

func (h ExecHost) DiskPresent(uuid string) bool {
	err := exec.Command(h.diskutil(), "info", uuid).Run()
	return err == nil
}

func (h ExecHost) VolumeMounted(name string) bool {
	out, err := exec.Command(h.mountBin()).Output()
	if err != nil {
		return false
	}
	needle := " on /Volumes/" + name + " "
	return strings.Contains(string(out), needle)
}

func (h ExecHost) Mount(uuid string) error {
	out, err := exec.Command(h.diskutil(), "mount", uuid).CombinedOutput()
	if err != nil {
		return fmt.Errorf("diskutil mount: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (h ExecHost) Unmount(uuid string) error {
	out, err := exec.Command(h.diskutil(), "unmount", uuid).CombinedOutput()
	if err != nil {
		return fmt.Errorf("diskutil unmount: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (h ExecHost) Status() (Status, error) {
	out, err := exec.Command(h.tmutil(), "status", "-X").Output()
	if err != nil {
		return Status{}, fmt.Errorf("tmutil status: %w", err)
	}
	return ParseStatus(out)
}

func (h ExecHost) StartBackup() (string, error) {
	cmd := exec.Command(h.tmutil(), "startbackup", "--block", "--auto")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func MountNeedle(name string) string {
	return " on /Volumes/" + name + " "
}

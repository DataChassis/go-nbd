package backend

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

type Ext4Backend struct {
	FilePath string
	FileSize int64
	fb       FileBackend
}

func (eb *Ext4Backend) ExecuteCmd(cmd string, args ...string) (string, error) {
	c := exec.Command(cmd, args...)
	var output strings.Builder
	c.Stdout = &output
	err := c.Run()
	return output.String(), err
}

func NewExt4Backend(filePath string, size int64) (Backend, error) {
	if filePath == "" || !strings.HasPrefix(filePath, "/") {
		return nil, fmt.Errorf("ext4 backend filename [%s] must be a valid absolute path", filePath)
	}
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0660)
	if err != nil {
		return nil, fmt.Errorf("failed to open/create ext4 backend file [%s]: %w", filePath, err)
	}
	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info for ext4 backend file [%s]: %w", filePath, err)
	}
	be := Ext4Backend{
		FilePath: filePath,
		FileSize: size,
		fb: FileBackend{
			file: file,
			lock: sync.RWMutex{},
		},
	}
	mkfs := false
	if stat.Size() < size {
		// if the file is zero length, it needs formatting, else resizing
		if stat.Size() == 0 {
			mkfs = true
		}
		// to make the file bigger, we seek to the last-but-one byte, then write a single zero byte
		if _, err = file.Seek(size-1, 0); err != nil {
			return nil, fmt.Errorf("failed to seek while extending backend file [%s]: %w", filePath, err)
		}
		if _, err = file.Write([]byte{0}); err != nil {
			return nil, fmt.Errorf("failed to write end byte while extending backend file [%s]: %w", filePath, err)
		}
		// flush the changes before we mount it
		if err = file.Sync(); err != nil {
			return nil, fmt.Errorf("failed to sync changes while extending backend file [%s]: %w", filePath, err)
		}
		// create a loop device that points to our file
		output, err := be.ExecuteCmd("udisksctl", "loop-setup", "--file", filePath)
		if err != nil || strings.HasPrefix(output, "Error ") {
			return nil, fmt.Errorf("udisks loop setup failed: %w : %s", err, output)
		}
		// loop device name is in the output (yeah...) so we need a regexp
		re, err := regexp.Compile("Mapped file " + filePath + " as (/dev/loop[0-9]{1,2})[.]")
		if err != nil {
			return nil, fmt.Errorf("failed to compile regexp to extract loop device name to create filesystem in [%s]: %w", filePath, err)
		}
		devName := ""
		if match := re.FindStringSubmatch(output); len(match) < 2 {
			return nil, fmt.Errorf("failed to find device name in Udisks loop setup output to create filesystem in [%s]: %w", filePath, err)
		} else {
			devName = match[1]
		}
		if mkfs {
			_, err = be.ExecuteCmd("mkfs.ext4", "-L", filePath, "-E", "root_owner=1000:1000", devName)
			if err != nil {
				return nil, fmt.Errorf("failed to make ext4 filesystem in [%s]: %w", filePath, err)
			}
		} else {
			_, err = be.ExecuteCmd("e2fsck", "-p", devName)
			if err != nil {
				return nil, fmt.Errorf("failed to run e2fsck against existing ext4 filesystem in [%s]: %w", filePath, err)
			}
			_, err = be.ExecuteCmd("resize2fs", devName)
			if err != nil {
				return nil, fmt.Errorf("failed to run resize2fs against existing ext4 filesystem in [%s]: %w", filePath, err)
			}
		}
		_, err = be.ExecuteCmd("udisksctl", "loop-delete", "-b", devName)
		if err != nil {
			return nil, fmt.Errorf("failed to unmount device [%s] extending filesystem in [%s]: %w", devName, filePath, err)
		}
	}
	return &be, nil
}

func (be *Ext4Backend) String() string { return fmt.Sprintf("ext4 backend, size=%d", be.FileSize) }

func (b *Ext4Backend) ReadAt(p []byte, off int64) (n int, err error) { return b.fb.ReadAt(p, off) }

func (b *Ext4Backend) WriteAt(p []byte, off int64) (n int, err error) { return b.fb.WriteAt(p, off) }

func (b *Ext4Backend) Size() (int64, error) { return b.fb.Size() }

func (b *Ext4Backend) Sync() error { return b.fb.Sync() }

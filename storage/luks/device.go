/*
Copyright 2026 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package luks

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/infinidat/infinibox-csi-driver/common"
)

// Block devices are inspected through sysfs, which the privileged node plugin shares with
// the node, and through the node's / mounted at /host. Tests point both elsewhere.
var (
	hostRoot = common.NodeRootDir
	sysRoot  = "/sys"

	// kernelNameOf returns the kernel name (dm-3, sdb, nvme0n1) of a block device of the node.
	kernelNameOf = kernelNameFromDeviceNode
)

// Mapping is a dm-crypt device, as the kernel reports it in sysfs.
type Mapping struct {
	Name              string // device-mapper name, e.g. infinibox-luks-12
	KernelName        string // e.g. dm-5
	BackingKernelName string // the device below it: dm-3 (multipath) or nvme0n1
	ReadOnly          bool
}

// DevicePath is the decrypted device on the node.
func (m Mapping) DevicePath() string {
	return "/dev/mapper/" + m.Name
}

// BackingDevicePath is the encrypted device below the mapping, on the node: /dev/mapper/<name>
// for a multipath device, /dev/<kernel name> otherwise.
func (m Mapping) BackingDevicePath() string {
	if name, err := readSysfs(m.BackingKernelName, "dm/name"); err == nil && name != "" {
		return "/dev/mapper/" + name
	}
	return "/dev/" + m.BackingKernelName
}

// MappingOf returns the dm-crypt mapping when device (a path on the node such as
// /dev/mapper/infinibox-luks-12 or /dev/dm-5) is one, and nil for any other device.
func MappingOf(device string) (*Mapping, error) {
	kernelName, err := kernelNameOf(device)
	if err != nil {
		return nil, err
	}
	return readCryptMapping(kernelName)
}

// Find returns the open dm-crypt device called name.
func Find(name string) (*Mapping, error) {
	m, isCrypt, err := findMapping(name)
	switch {
	case err != nil:
		return nil, err
	case m == nil:
		return nil, common.Errorf("LUKS device %s is not open on this node", name)
	case !isCrypt:
		return nil, common.Errorf("device-mapper device %s is not a dm-crypt device", name)
	}
	return m, nil
}

// findMapping returns the device-mapper device called name, or nil if there is none.
// isCrypt tells whether it is a dm-crypt device.
func findMapping(name string) (mapping *Mapping, isCrypt bool, err error) {
	files, err := filepath.Glob(filepath.Join(sysRoot, "block", "dm-*", "dm", "name"))
	if err != nil {
		return nil, false, common.Errorf("%w", err)
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil || strings.TrimSpace(string(content)) != name {
			continue
		}
		kernelName := filepath.Base(filepath.Dir(filepath.Dir(file)))
		m, err := readCryptMapping(kernelName)
		if err != nil {
			return nil, false, err
		}
		if m == nil {
			return &Mapping{Name: name, KernelName: kernelName}, false, nil
		}
		return m, true, nil
	}
	return nil, false, nil
}

func readCryptMapping(kernelName string) (*Mapping, error) {
	uuid, err := readSysfs(kernelName, "dm/uuid")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil // not a device-mapper device
	}
	if err != nil {
		return nil, common.Errorf("%w", err)
	}
	if !strings.HasPrefix(uuid, "CRYPT-") {
		return nil, nil
	}
	name, err := readSysfs(kernelName, "dm/name")
	if err != nil {
		return nil, common.Errorf("%w", err)
	}
	slaves, err := os.ReadDir(filepath.Join(sysRoot, "block", kernelName, "slaves"))
	if err != nil {
		return nil, common.Errorf("%w", err)
	}
	if len(slaves) != 1 {
		return nil, common.Errorf("dm-crypt device %s (%s) has %d devices below it, expected 1", name, kernelName, len(slaves))
	}
	ro, _ := readSysfs(kernelName, "ro")
	return &Mapping{
		Name:              name,
		KernelName:        kernelName,
		BackingKernelName: slaves[0].Name(),
		ReadOnly:          ro == "1",
	}, nil
}

// sizeBytes reads the size of a block device from sysfs, which shows a resize at once.
func sizeBytes(kernelName string) (int64, error) {
	sectors, err := readSysfs(kernelName, "size")
	if err != nil {
		return 0, common.Errorf("%w", err)
	}
	n, err := strconv.ParseInt(sectors, 10, 64)
	if err != nil {
		return 0, common.Errorf("size of %s: %w", kernelName, err)
	}
	return n * 512, nil // always in 512-byte sectors
}

func readSysfs(kernelName, attribute string) (string, error) {
	content, err := os.ReadFile(filepath.Join(sysRoot, "block", kernelName, attribute))
	return strings.TrimSpace(string(content)), err
}

func kernelNameFromDeviceNode(device string) (string, error) {
	info, err := os.Stat(filepath.Join(hostRoot, device))
	if err != nil {
		return "", common.Errorf("%w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice != 0 {
		return "", common.Errorf("%s is not a block device", device)
	}
	rdev := uint64(stat.Rdev) // Rdev is not uint64 on every platform
	majorMinor := fmt.Sprintf("%d:%d", major(rdev), minor(rdev))
	link, err := filepath.EvalSymlinks(filepath.Join(sysRoot, "dev", "block", majorMinor))
	if err != nil {
		return "", common.Errorf("%s (%s): %w", device, majorMinor, err)
	}
	return filepath.Base(link), nil
}

// Linux dev_t encoding, see gnu_dev_major and gnu_dev_minor.
func major(dev uint64) uint64 { return (dev>>8)&0xfff | (dev>>32)&^0xfff }
func minor(dev uint64) uint64 { return dev&0xff | (dev>>12)&^0xff }

// blankCheckBytes is how much of the head and of the tail of a device must read as zero
// before it is formatted. A new InfiniBox volume reads as zero until it is written, so
// this catches data that has no signature blkid knows.
const blankCheckBytes = 8 << 20

// nonZeroEnds explains why the head or tail of the device are not all zero, or returns "".
func nonZeroEnds(path string, n int64) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- a device the driver attached to this node
	if err != nil {
		return "", common.Errorf("%w", err)
	}
	defer func() { _ = f.Close() }()

	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return "", common.Errorf("%w", err)
	}
	n = min(n, size)
	for _, offset := range []int64{0, size - n} {
		zero, err := allZero(io.NewSectionReader(f, offset, n))
		if err != nil {
			return "", common.Errorf("%w", err)
		}
		if !zero {
			return fmt.Sprintf("%d bytes at offset %d are not all zero (data without a known signature?)", n, offset), nil
		}
	}
	return "", nil
}

func allZero(r io.Reader) (bool, error) {
	buf := make([]byte, 1<<20)
	zero := make([]byte, len(buf))
	for {
		n, err := r.Read(buf)
		if !bytes.Equal(buf[:n], zero[:n]) {
			return false, nil
		}
		if err == io.EOF {
			return true, nil
		}
		if err != nil {
			return false, err
		}
	}
}

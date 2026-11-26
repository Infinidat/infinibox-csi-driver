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

// Package luks encrypts block volumes on the node with LUKS2 (dm-crypt).
//
// The encrypted device is the volume attached to the node (multipath or NVMe device);
// the decrypted device is /dev/mapper/infinibox-luks-<volume id>, and the filesystem (or
// the raw block device given to the pod) is on top of it. cryptsetup runs on the node,
// like the other storage tools of the driver (see host-chroot.sh), and the passphrase
// always goes to cryptsetup through stdin: never on a command line, in a log or in a file.
//
// Safety rules:
//   - a device is formatted only if it is blank: no signature found by blkid and zeroes at
//     its head and tail. A device holding data is never formatted;
//   - a mapping already open on the node is reused only if it is a dm-crypt device on the
//     same device and the passphrase opens the LUKS header, so a mapping left open never
//     lets a wrong passphrase through.
package luks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/infinidat/infinibox-csi-driver/common"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	mapperPrefix = "infinibox-luks-"

	cipher         = "aes-xts-plain64"
	keySizeBits    = "512"
	pbkdf          = "argon2id"
	defaultMemKiB  = 262144 // 256 MiB
	EnvPBKDFMemory = "LUKS_PBKDF_MEMORY_KIB"

	// cryptsetup exit codes, see RETURN CODES in man cryptsetup
	exitWrongPassphrase = 2
	exitOutOfMemory     = 3
	exitBusy            = 5

	// blkid exit code when it finds nothing on the device
	blkidNothingFound = 2
)

var (
	// resolved in PATH: in the driver image, /ibox/cryptsetup and /ibox/blkid run the node's tools
	cryptsetupCommand = "cryptsetup"
	blkidCommand      = "blkid"

	// argon2id uses LUKS_PBKDF_MEMORY_KIB of memory for each key derivation (format, open,
	// passphrase test, resize), charged to the node plugin container: one at a time keeps
	// the container under its memory limit.
	pbkdfLock sync.Mutex

	// time spent deriving the key, 0 for the cryptsetup default (2 s); tests lower it
	iterTimeMs = 0

	pollInterval = time.Second
	mapperWait   = 10 * time.Second
	growWait     = 60 * time.Second
)

// MapperName is the device-mapper name of the decrypted device of a volume.
func MapperName(volumeID int) string {
	return mapperPrefix + strconv.Itoa(volumeID)
}

// OpenOptions describe the volume to open.
type OpenOptions struct {
	Device     string // encrypted device on the node, e.g. /dev/mapper/mpatha or /dev/nvme0n1
	Name       string // device-mapper name of the decrypted device, see MapperName
	Passphrase []byte
	ReadOnly   bool
}

// Open makes the decrypted device of a volume available on the node and returns its path,
// /dev/mapper/<Name>. A blank device is formatted with LUKS2 first. Open can run again
// for a volume that is already open (another pod on the same node, a retried call).
func Open(ctx context.Context, o OpenOptions) (string, error) {
	if len(o.Passphrase) == 0 {
		return "", common.Errorf("empty passphrase")
	}
	existing, isCrypt, err := findMapping(o.Name)
	if err != nil {
		return "", err
	}
	if existing != nil {
		if err := checkReusable(ctx, o, existing, isCrypt); err != nil {
			return "", err
		}
		slog.Info("reusing the LUKS device already open on this node", "name", o.Name, "device", o.Device)
		return waitForMapperNode(o.Name)
	}

	isLUKS, err := isLuks(ctx, o.Device)
	if err != nil {
		return "", err
	}
	if !isLUKS {
		if err := formatIfBlank(ctx, o); err != nil {
			return "", err
		}
	}

	args := []string{"open", "--type", "luks", "--key-file", "-"}
	if o.ReadOnly {
		args = append(args, "--readonly")
	}
	args = append(args, o.Device, o.Name)
	slog.Info("opening LUKS device", "device", o.Device, "name", o.Name, "readOnly", o.ReadOnly)
	if err := cryptsetupWithPassphrase(ctx, o.Passphrase, args...); err != nil {
		return "", openError(o, err)
	}
	return waitForMapperNode(o.Name)
}

func checkReusable(ctx context.Context, o OpenOptions, m *Mapping, isCrypt bool) error {
	if !isCrypt {
		return status.Error(codes.FailedPrecondition, common.Errorf(
			"a device-mapper device named %s already exists on this node and it is not a dm-crypt device: refusing to use it", o.Name).Error())
	}
	deviceKernelName, err := kernelNameOf(o.Device)
	if err != nil {
		return err
	}
	if m.BackingKernelName != deviceKernelName {
		return status.Error(codes.FailedPrecondition, common.Errorf(
			"%s is already open on this node on device %s, not on %s (%s): refusing to use it. "+
				"If no pod uses it, close it on the node with 'cryptsetup close %s'",
			o.Name, m.BackingKernelName, o.Device, deviceKernelName, o.Name).Error())
	}
	if m.ReadOnly && !o.ReadOnly {
		return status.Error(codes.FailedPrecondition, common.Errorf(
			"%s is already open read-only on this node, it cannot be used read-write", o.Name).Error())
	}
	// a mapping left open must not let a wrong passphrase through
	err = cryptsetupWithPassphrase(ctx, o.Passphrase, "open", "--test-passphrase", "--type", "luks", "--key-file", "-", o.Device)
	if err != nil {
		return openError(o, err)
	}
	return nil
}

func formatIfBlank(ctx context.Context, o OpenOptions) error {
	reason, err := notBlankReason(ctx, o.Device)
	if err != nil {
		return err
	}
	if reason != "" {
		return status.Error(codes.FailedPrecondition, common.Errorf(
			"the volume is encrypted but device %s is not LUKS and is not blank (%s): refusing to format it, the device was not modified",
			o.Device, reason).Error())
	}
	if o.ReadOnly {
		return status.Error(codes.FailedPrecondition, common.Errorf(
			"device %s is blank and published read-only: it cannot be formatted with LUKS", o.Device).Error())
	}

	slog.Info("formatting blank device with LUKS2", "device", o.Device, "cipher", cipher, "keySize", keySizeBits,
		"pbkdf", pbkdf, "pbkdfMemoryKiB", pbkdfMemoryKiB())
	args := []string{"luksFormat", "--batch-mode", "--type", "luks2",
		"--cipher", cipher, "--key-size", keySizeBits, "--hash", "sha256",
		"--pbkdf", pbkdf, "--pbkdf-memory", strconv.Itoa(pbkdfMemoryKiB())}
	if iterTimeMs > 0 {
		args = append(args, "--iter-time", strconv.Itoa(iterTimeMs))
	}
	args = append(args, "--key-file", "-", o.Device)
	err = cryptsetupWithPassphrase(ctx, o.Passphrase, args...)
	if err != nil {
		return common.Errorf("luksFormat of %s failed%s: %w", o.Device, memoryHint(err), err)
	}
	return nil
}

// notBlankReason explains why the device must not be formatted, or returns "" if it is blank.
func notBlankReason(ctx context.Context, device string) (string, error) {
	// low-level probe: filesystems, partition tables, RAID, LVM, LUKS...
	res, err := run(ctx, nil, blkidCommand, "-p", "-o", "export", device)
	if err != nil {
		return "", err
	}
	switch res.exitCode {
	case 0:
		return "blkid found " + strings.Join(strings.Fields(res.stdout), " "), nil
	case blkidNothingFound:
	default:
		return fmt.Sprintf("blkid probe failed (exit code %d): %s", res.exitCode, strings.TrimSpace(res.stderr)), nil
	}
	return nonZeroEnds(filepath.Join(hostRoot, device), blankCheckBytes)
}

func isLuks(ctx context.Context, device string) (bool, error) {
	res, err := run(ctx, nil, cryptsetupCommand, "isLuks", device)
	if err != nil {
		return false, err
	}
	switch res.exitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, common.Errorf("cryptsetup isLuks %s: exit code %d: %s", device, res.exitCode, strings.TrimSpace(res.stderr))
	}
}

// Close removes the decrypted device of a volume from the node. It does nothing if the
// device is not open, and fails if it is still in use.
func Close(ctx context.Context, name string) error {
	m, isCrypt, err := findMapping(name)
	if err != nil || m == nil {
		return err
	}
	if !isCrypt {
		return common.Errorf("device-mapper device %s is not a dm-crypt device: not closing it", name)
	}
	slog.Info("closing LUKS device", "name", name)
	res, err := run(ctx, nil, cryptsetupCommand, "close", name)
	if err != nil {
		return err
	}
	switch res.exitCode {
	case 0:
		return nil
	case exitBusy:
		return common.Errorf("LUKS device %s is still in use (mounted?), it cannot be closed: %s", name, strings.TrimSpace(res.stderr))
	default:
		return common.Errorf("cryptsetup close %s: exit code %d: %s", name, res.exitCode, strings.TrimSpace(res.stderr))
	}
}

// Resize grows an open LUKS device to the size of the device below it, after waiting
// until that device is at least minBytes large (0: do not wait). With LUKS2 the volume
// key is in the kernel keyring, so cryptsetup needs the passphrase to resize.
func Resize(ctx context.Context, m Mapping, passphrase []byte, minBytes int64) error {
	if minBytes > 0 {
		if err := waitForSize(ctx, m.BackingKernelName, minBytes); err != nil {
			return err
		}
	}
	slog.Info("resizing LUKS device", "name", m.Name, "device", m.BackingDevicePath())
	err := cryptsetupWithPassphrase(ctx, passphrase, "resize", "--key-file", "-", m.Name)
	if err != nil {
		return common.Errorf("cryptsetup resize %s failed%s: %w", m.Name, memoryHint(err), err)
	}
	return nil
}

func waitForSize(ctx context.Context, kernelName string, minBytes int64) error {
	deadline := time.Now().Add(growWait)
	for {
		size, err := sizeBytes(kernelName)
		if err != nil {
			return err
		}
		if size >= minBytes {
			return nil
		}
		if time.Now().After(deadline) {
			return common.Errorf("device %s is %d bytes, it has not grown to %d bytes yet", kernelName, size, minBytes)
		}
		select {
		case <-ctx.Done():
			return common.Errorf("%w", ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// waitForMapperNode waits for udev on the node to create /dev/mapper/<name>.
func waitForMapperNode(name string) (string, error) {
	devicePath := "/dev/mapper/" + name
	deadline := time.Now().Add(mapperWait)
	for {
		if _, err := os.Stat(filepath.Join(hostRoot, devicePath)); err == nil {
			return devicePath, nil
		}
		if time.Now().After(deadline) {
			return "", common.Errorf("%s did not appear on the node", devicePath)
		}
		time.Sleep(pollInterval)
	}
}

func openError(o OpenOptions, err error) error {
	var exitErr *cryptsetupExitError
	if errors.As(err, &exitErr) {
		switch exitErr.code {
		case exitWrongPassphrase:
			return status.Error(codes.PermissionDenied, common.Errorf(
				"the passphrase does not open the LUKS device %s, the device was not modified: check the encryption Secret", o.Device).Error())
		case exitBusy:
			return status.Error(codes.FailedPrecondition, common.Errorf(
				"cannot open %s as %s: the device or the name is already in use: %w", o.Device, o.Name, err).Error())
		}
	}
	return common.Errorf("cannot open the LUKS device %s%s: %w", o.Device, memoryHint(err), err)
}

func memoryHint(err error) string {
	var exitErr *cryptsetupExitError
	if (errors.As(err, &exitErr) && exitErr.code == exitOutOfMemory) || strings.Contains(err.Error(), "signal: killed") {
		return fmt.Sprintf(" (out of memory: the node plugin memory limit must be well above %s=%d)", EnvPBKDFMemory, pbkdfMemoryKiB())
	}
	return ""
}

func pbkdfMemoryKiB() int {
	value := os.Getenv(EnvPBKDFMemory)
	if value == "" {
		return defaultMemKiB
	}
	kib, err := strconv.Atoi(value)
	if err != nil || kib <= 0 {
		slog.Error("invalid value, using the default", "env var", EnvPBKDFMemory, "value", value, "default", defaultMemKiB)
		return defaultMemKiB
	}
	return kib
}

type cryptsetupExitError struct {
	code   int
	stderr string
}

func (e *cryptsetupExitError) Error() string {
	return fmt.Sprintf("cryptsetup exit code %d: %s", e.code, e.stderr)
}

// cryptsetupWithPassphrase runs a cryptsetup command that derives a key from the
// passphrase, which it reads from stdin (--key-file -).
func cryptsetupWithPassphrase(ctx context.Context, passphrase []byte, args ...string) error {
	pbkdfLock.Lock()
	defer pbkdfLock.Unlock()
	res, err := run(ctx, passphrase, cryptsetupCommand, args...)
	if err != nil {
		return err
	}
	if res.exitCode != 0 {
		return &cryptsetupExitError{code: res.exitCode, stderr: strings.TrimSpace(res.stderr)}
	}
	return nil
}

type result struct {
	stdout   string
	stderr   string
	exitCode int
}

// run runs a command without a shell. A non-zero exit code is not an error: the caller
// decides what it means. stdin may hold the passphrase, so it is never logged.
func run(ctx context.Context, stdin []byte, command string, args ...string) (result, error) {
	slog.Debug("running", "command", command, "args", args)
	cmd := exec.CommandContext(ctx, command, args...) // #nosec G204 -- fixed commands, arguments are device paths and names built by the driver
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := result{stdout: stdout.String(), stderr: stderr.String()}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.Exited() {
		res.exitCode = exitErr.ExitCode()
		return res, nil
	}
	if err != nil {
		// not started, or killed by a signal (e.g. the OOM killer)
		return res, common.Errorf("%s %s: %w %s", command, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return res, nil
}

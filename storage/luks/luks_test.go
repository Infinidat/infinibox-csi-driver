//go:build unit

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
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	testDevice     = "/dev/mapper/mpatha" // the multipath device of the volume, on the node
	testDeviceDM   = "dm-3"
	testName       = "infinibox-luks-42"
	testPassphrase = "s3cret passphrase"
)

// fakeNode is a node with a fake sysfs, a fake /host, and fake cryptsetup and blkid that
// log their arguments and what they read on stdin.
type fakeNode struct {
	t    *testing.T
	sys  string
	host string
	log  string
}

const fakeCryptsetup = `#!/bin/sh
printf 'args=%s\n' "$*" >> "$FAKE_LOG"
case "$1 $2" in
  "luksFormat "*|"open "*|"resize "*) printf 'stdin=%s\n' "$(cat)" >> "$FAKE_LOG" ;;
esac
case "$1" in
  isLuks) exit "${FAKE_ISLUKS:-1}" ;;
  luksFormat) exit "${FAKE_FORMAT:-0}" ;;
  open)
    if [ "$2" = "--test-passphrase" ]; then exit "${FAKE_TEST:-0}"; fi
    [ "${FAKE_OPEN:-0}" = 0 ] && mkdir -p "$FAKE_HOST/dev/mapper" && touch "$FAKE_HOST/dev/mapper/$(eval echo \${$#})"
    exit "${FAKE_OPEN:-0}" ;;
  close) exit "${FAKE_CLOSE:-0}" ;;
  resize) exit "${FAKE_RESIZE:-0}" ;;
esac
exit 99
`

const fakeBlkid = `#!/bin/sh
printf 'blkid=%s\n' "$*" >> "$FAKE_LOG"
[ -n "$FAKE_BLKID_OUT" ] && echo "$FAKE_BLKID_OUT"
exit "${FAKE_BLKID:-2}"
`

func newFakeNode(t *testing.T) *fakeNode {
	n := &fakeNode{t: t, sys: t.TempDir(), host: t.TempDir()}
	bin := t.TempDir()
	n.log = filepath.Join(bin, "log")
	writeFile(t, filepath.Join(bin, "cryptsetup"), fakeCryptsetup, 0o755)
	writeFile(t, filepath.Join(bin, "blkid"), fakeBlkid, 0o755)
	t.Setenv("FAKE_LOG", n.log)
	t.Setenv("FAKE_HOST", n.host)

	saved := []any{hostRoot, sysRoot, kernelNameOf, cryptsetupCommand, blkidCommand, pollInterval, mapperWait, growWait}
	t.Cleanup(func() {
		hostRoot, sysRoot = saved[0].(string), saved[1].(string)
		kernelNameOf = saved[2].(func(string) (string, error))
		cryptsetupCommand, blkidCommand = saved[3].(string), saved[4].(string)
		pollInterval, mapperWait, growWait = saved[5].(time.Duration), saved[6].(time.Duration), saved[7].(time.Duration)
	})
	hostRoot, sysRoot = n.host, n.sys
	cryptsetupCommand, blkidCommand = filepath.Join(bin, "cryptsetup"), filepath.Join(bin, "blkid")
	pollInterval, mapperWait, growWait = time.Millisecond, 50*time.Millisecond, 50*time.Millisecond
	kernelNameOf = func(device string) (string, error) {
		switch device {
		case testDevice:
			return testDeviceDM, nil
		case "/dev/mapper/" + testName:
			return "dm-5", nil
		}
		return "", errors.New("unknown device " + device)
	}

	// the multipath device: a blank 32 MiB volume
	n.addBlockDevice(testDeviceDM, "mpath-36742b0f0000004d2000000000000abcd", "mpatha", 32<<20)
	require.NoError(t, os.MkdirAll(filepath.Join(n.host, filepath.Dir(testDevice)), 0o755))
	f, err := os.Create(filepath.Join(n.host, testDevice))
	require.NoError(t, err)
	require.NoError(t, f.Truncate(32<<20))
	require.NoError(t, f.Close())
	return n
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
}

// addBlockDevice adds a device-mapper device to the fake sysfs.
func (n *fakeNode) addBlockDevice(kernelName, uuid, name string, size int64, slaves ...string) {
	dir := filepath.Join(n.sys, "block", kernelName)
	writeFile(n.t, filepath.Join(dir, "dm", "uuid"), uuid+"\n", 0o644)
	writeFile(n.t, filepath.Join(dir, "dm", "name"), name+"\n", 0o644)
	writeFile(n.t, filepath.Join(dir, "size"), itoa(size/512)+"\n", 0o644)
	writeFile(n.t, filepath.Join(dir, "ro"), "0\n", 0o644)
	require.NoError(n.t, os.MkdirAll(filepath.Join(dir, "slaves"), 0o755))
	for _, slave := range slaves {
		writeFile(n.t, filepath.Join(dir, "slaves", slave), "", 0o644)
	}
}

// openMapping simulates a LUKS device already open on the node.
func (n *fakeNode) openMapping(backing string) {
	n.addBlockDevice("dm-5", "CRYPT-LUKS2-0123456789abcdef0123456789abcdef-"+testName, testName, 16<<20, backing)
	writeFile(n.t, filepath.Join(n.host, "dev", "mapper", testName), "", 0o644)
}

func (n *fakeNode) calls() string {
	content, err := os.ReadFile(n.log)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	require.NoError(n.t, err)
	return string(content)
}

func itoa(i int64) string {
	return strconv.FormatInt(i, 10)
}

func openOptions() OpenOptions {
	return OpenOptions{Device: testDevice, Name: testName, Passphrase: []byte(testPassphrase)}
}

func TestMapperName(t *testing.T) {
	assert.Equal(t, "infinibox-luks-12345", MapperName(12345))
}

func TestOpenFormatsBlankDevice(t *testing.T) {
	n := newFakeNode(t)
	path, err := Open(t.Context(), openOptions())
	require.NoError(t, err)
	assert.Equal(t, "/dev/mapper/"+testName, path)

	calls := n.calls()
	assert.Contains(t, calls, "args=isLuks "+testDevice)
	assert.Contains(t, calls, "blkid=-p -o export "+testDevice)
	assert.Contains(t, calls, "args=luksFormat --batch-mode --type luks2 --cipher aes-xts-plain64 --key-size 512 --hash sha256 --pbkdf argon2id --pbkdf-memory 262144 --key-file - "+testDevice)
	assert.Contains(t, calls, "args=open --type luks --key-file - "+testDevice+" "+testName)
	assert.Equal(t, 2, strings.Count(calls, "stdin="+testPassphrase), "format and open read the passphrase on stdin")
	for _, line := range strings.Split(calls, "\n") {
		if strings.HasPrefix(line, "args=") {
			assert.NotContains(t, line, testPassphrase, "the passphrase must never be on the command line")
		}
	}
	assert.Less(t, strings.Index(calls, "luksFormat"), strings.Index(calls, "args=open"))
}

func TestOpenPBKDFMemoryFromEnv(t *testing.T) {
	n := newFakeNode(t)
	t.Setenv(EnvPBKDFMemory, "65536")
	_, err := Open(t.Context(), openOptions())
	require.NoError(t, err)
	assert.Contains(t, n.calls(), "--pbkdf-memory 65536 ")
}

func TestOpenExistingLUKSDoesNotFormat(t *testing.T) {
	n := newFakeNode(t)
	t.Setenv("FAKE_ISLUKS", "0")
	_, err := Open(t.Context(), openOptions())
	require.NoError(t, err)
	calls := n.calls()
	assert.NotContains(t, calls, "luksFormat")
	assert.NotContains(t, calls, "blkid=")
	assert.Contains(t, calls, "args=open --type luks --key-file - "+testDevice+" "+testName)
}

func TestOpenReadOnly(t *testing.T) {
	n := newFakeNode(t)
	t.Setenv("FAKE_ISLUKS", "0")
	o := openOptions()
	o.ReadOnly = true
	_, err := Open(t.Context(), o)
	require.NoError(t, err)
	assert.Contains(t, n.calls(), "args=open --type luks --key-file - --readonly "+testDevice+" "+testName)
}

func TestOpenRefusesToFormatReadOnlyBlankDevice(t *testing.T) {
	n := newFakeNode(t)
	o := openOptions()
	o.ReadOnly = true
	_, err := Open(t.Context(), o)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.NotContains(t, n.calls(), "luksFormat")
}

func TestOpenRefusesDeviceWithSignature(t *testing.T) {
	n := newFakeNode(t)
	t.Setenv("FAKE_BLKID", "0")
	t.Setenv("FAKE_BLKID_OUT", "TYPE=ext4")
	_, err := Open(t.Context(), openOptions())
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Contains(t, err.Error(), "TYPE=ext4")
	assert.NotContains(t, n.calls(), "luksFormat")
	assert.NotContains(t, n.calls(), "args=open")
}

func TestOpenRefusesDeviceWithDataWithoutSignature(t *testing.T) {
	for name, offset := range map[string]int64{"head": 4096, "tail": 32<<20 - 1} {
		t.Run(name, func(t *testing.T) {
			n := newFakeNode(t)
			f, err := os.OpenFile(filepath.Join(n.host, testDevice), os.O_WRONLY, 0)
			require.NoError(t, err)
			_, err = f.WriteAt([]byte{1}, offset)
			require.NoError(t, err)
			require.NoError(t, f.Close())

			_, err = Open(t.Context(), openOptions())
			assert.Equal(t, codes.FailedPrecondition, status.Code(err))
			assert.Contains(t, err.Error(), "not all zero")
			assert.NotContains(t, n.calls(), "luksFormat")
		})
	}
}

func TestOpenWrongPassphrase(t *testing.T) {
	newFakeNode(t)
	t.Setenv("FAKE_ISLUKS", "0")
	t.Setenv("FAKE_OPEN", "2")
	_, err := Open(t.Context(), openOptions())
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Contains(t, err.Error(), "not modified")
}

func TestOpenOutOfMemoryHint(t *testing.T) {
	newFakeNode(t)
	t.Setenv("FAKE_ISLUKS", "0")
	t.Setenv("FAKE_OPEN", "3")
	_, err := Open(t.Context(), openOptions())
	require.Error(t, err)
	assert.Contains(t, err.Error(), EnvPBKDFMemory)
}

func TestOpenReusesMappingAfterTestingPassphrase(t *testing.T) {
	n := newFakeNode(t)
	n.openMapping(testDeviceDM)
	path, err := Open(t.Context(), openOptions())
	require.NoError(t, err)
	assert.Equal(t, "/dev/mapper/"+testName, path)
	calls := n.calls()
	assert.Contains(t, calls, "args=open --test-passphrase --type luks --key-file - "+testDevice)
	assert.Contains(t, calls, "stdin="+testPassphrase)
	assert.NotContains(t, calls, "isLuks")
	assert.NotContains(t, calls, "args=open --type")
}

func TestOpenOpenMappingDoesNotSkipPassphrase(t *testing.T) {
	n := newFakeNode(t)
	n.openMapping(testDeviceDM)
	t.Setenv("FAKE_TEST", "2")
	_, err := Open(t.Context(), openOptions())
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestOpenRefusesMappingOnAnotherDevice(t *testing.T) {
	n := newFakeNode(t)
	n.openMapping("dm-9")
	_, err := Open(t.Context(), openOptions())
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Contains(t, err.Error(), "dm-9")
	assert.Empty(t, n.calls())
}

func TestOpenRefusesMappingThatIsNotCrypt(t *testing.T) {
	n := newFakeNode(t)
	n.addBlockDevice("dm-5", "LVM-abcdef", testName, 16<<20, testDeviceDM)
	_, err := Open(t.Context(), openOptions())
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Empty(t, n.calls())
}

func TestOpenReadWriteOnReadOnlyMapping(t *testing.T) {
	n := newFakeNode(t)
	n.openMapping(testDeviceDM)
	writeFile(t, filepath.Join(n.sys, "block", "dm-5", "ro"), "1\n", 0o644)
	_, err := Open(t.Context(), openOptions())
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestClose(t *testing.T) {
	n := newFakeNode(t)
	require.NoError(t, Close(t.Context(), testName), "nothing to close")
	assert.Empty(t, n.calls())

	n.openMapping(testDeviceDM)
	require.NoError(t, Close(t.Context(), testName))
	assert.Contains(t, n.calls(), "args=close "+testName)

	t.Setenv("FAKE_CLOSE", "5")
	err := Close(t.Context(), testName)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "still in use")
}

func TestCloseRefusesMappingThatIsNotCrypt(t *testing.T) {
	n := newFakeNode(t)
	n.addBlockDevice("dm-5", "LVM-abcdef", testName, 16<<20, testDeviceDM)
	assert.Error(t, Close(t.Context(), testName))
	assert.Empty(t, n.calls())
}

func TestFindAndMappingOf(t *testing.T) {
	n := newFakeNode(t)
	_, err := Find(testName)
	assert.Error(t, err, "not open")

	n.openMapping(testDeviceDM)
	m, err := Find(testName)
	require.NoError(t, err)
	assert.Equal(t, Mapping{Name: testName, KernelName: "dm-5", BackingKernelName: testDeviceDM}, *m)
	assert.Equal(t, "/dev/mapper/"+testName, m.DevicePath())
	assert.Equal(t, "/dev/mapper/mpatha", m.BackingDevicePath())

	m, err = MappingOf("/dev/mapper/" + testName)
	require.NoError(t, err)
	assert.Equal(t, testName, m.Name)

	m, err = MappingOf(testDevice)
	require.NoError(t, err)
	assert.Nil(t, m, "a multipath device is not a LUKS device")

	nvme := Mapping{BackingKernelName: "nvme0n1"}
	assert.Equal(t, "/dev/nvme0n1", nvme.BackingDevicePath())
}

func TestResizeWaitsForTheDeviceToGrow(t *testing.T) {
	n := newFakeNode(t)
	n.openMapping(testDeviceDM)
	m, err := Find(testName)
	require.NoError(t, err)

	err = Resize(t.Context(), *m, []byte(testPassphrase), 64<<20)
	require.Error(t, err, "the device is still 32 MiB")
	assert.Contains(t, err.Error(), "has not grown")
	assert.NotContains(t, n.calls(), "resize")

	writeFile(t, filepath.Join(n.sys, "block", testDeviceDM, "size"), itoa(64<<20/512)+"\n", 0o644)
	require.NoError(t, Resize(t.Context(), *m, []byte(testPassphrase), 64<<20))
	assert.Contains(t, n.calls(), "args=resize --key-file - "+testName)
	assert.Contains(t, n.calls(), "stdin="+testPassphrase)
}

func TestNonZeroEnds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk")
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(1<<20))
	require.NoError(t, f.Close())

	reason, err := nonZeroEnds(path, 4096)
	require.NoError(t, err)
	assert.Empty(t, reason)

	// smaller than the area to check
	reason, err = nonZeroEnds(path, 8<<20)
	require.NoError(t, err)
	assert.Empty(t, reason)

	require.NoError(t, os.WriteFile(path, []byte("data"), 0o644))
	reason, err = nonZeroEnds(path, 4096)
	require.NoError(t, err)
	assert.NotEmpty(t, reason)
}

// TestRealCryptsetup formats an image file with the real cryptsetup, which does not need
// device-mapper: it checks the arguments and that the passphrase on stdin is used as is.
func TestRealCryptsetup(t *testing.T) {
	cryptsetup, err := exec.LookPath("cryptsetup")
	if err != nil {
		t.Skip("cryptsetup not installed")
	}
	blkid, err := exec.LookPath("blkid")
	if err != nil {
		t.Skip("blkid not installed")
	}
	savedRoot, savedCryptsetup, savedBlkid := hostRoot, cryptsetupCommand, blkidCommand
	t.Cleanup(func() { hostRoot, cryptsetupCommand, blkidCommand = savedRoot, savedCryptsetup, savedBlkid })
	hostRoot, cryptsetupCommand, blkidCommand = "/", cryptsetup, blkid
	t.Setenv(EnvPBKDFMemory, "32768") // keep the test fast
	savedIterTime := iterTimeMs
	t.Cleanup(func() { iterTimeMs = savedIterTime })
	iterTimeMs = 100

	image := filepath.Join(t.TempDir(), "volume.img")
	f, err := os.Create(image)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(64<<20))
	require.NoError(t, f.Close())

	o := OpenOptions{Device: image, Name: "unused", Passphrase: []byte(testPassphrase + "\n")}
	isLUKS, err := isLuks(t.Context(), image)
	require.NoError(t, err)
	assert.False(t, isLUKS)
	require.NoError(t, formatIfBlank(t.Context(), o))

	isLUKS, err = isLuks(t.Context(), image)
	require.NoError(t, err)
	assert.True(t, isLUKS)

	test := func(passphrase string) error {
		return cryptsetupWithPassphrase(t.Context(), []byte(passphrase), "open", "--test-passphrase", "--type", "luks", "--key-file", "-", image)
	}
	assert.NoError(t, test(testPassphrase+"\n"))
	err = test(testPassphrase)
	assert.Equal(t, codes.PermissionDenied, status.Code(openError(o, err)), "the newline is part of the passphrase")

	reason, err := notBlankReason(t.Context(), image)
	require.NoError(t, err)
	assert.Contains(t, reason, "crypto_LUKS", "a LUKS device is never blank")

	err = formatIfBlank(t.Context(), o)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err), "never format twice")
}

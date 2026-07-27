package apply

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

const testRoverID = "sim-01"

// writeFixtures creates fake /proc/cmdline and /etc/waypoint/image.toml
// files for swupdateSelector() to read. partition must be "A" or "B".
func writeFixtures(t *testing.T, partition, variant string) (cmdline, imageTOML string) {
	t.Helper()
	dir := t.TempDir()

	var uuid string
	switch partition {
	case "A":
		uuid = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	case "B":
		uuid = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	default:
		t.Fatalf("writeFixtures: bad partition %q", partition)
	}

	cmdline = filepath.Join(dir, "cmdline")
	if err := os.WriteFile(cmdline, []byte("root=PARTUUID="+uuid+" rootfstype=squashfs ro\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	imageTOML = filepath.Join(dir, "image.toml")
	content := fmt.Sprintf("[image]\nversion = \"0.5.0\"\nvariant = %q\npartition = \"A\"\n", variant)
	if err := os.WriteFile(imageTOML, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return cmdline, imageTOML
}

// fakeVerifier is a test double for verify.Verifier. The zero value passes
// every verification; set err to make Verify fail. calls records the number
// of invocations so tests can assert verification was (or wasn't) attempted.
type fakeVerifier struct {
	err   error
	calls atomic.Int32
}

func (f *fakeVerifier) Verify(_ context.Context, _, _ string) error {
	f.calls.Add(1)
	return f.err
}

// natsTestConn starts an in-process NATS server and returns a connection plus
// a cleanup func. Mirrors the pattern used in agent/internal/cameras tests.
func natsTestConn(t *testing.T) (*nats.Conn, func()) {
	t.Helper()
	s, err := server.NewServer(&server.Options{Port: -1})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	if !s.ReadyForConnections(2 * time.Second) {
		s.Shutdown()
		t.Fatal("nats server not ready")
	}
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		s.Shutdown()
		t.Fatal(err)
	}
	return nc, func() {
		nc.Close()
		s.Shutdown()
	}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// artifactServer serves both <path> (artifact bytes) and <path>.cosign
// (bundle bytes) so tests can exercise the full download+verify dance
// without a real sigstore bundle.
func artifactServer(payload, bundle []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".cosign") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(bundle)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	}))
}

// failureCatcher returns an OnApplyFailed hook and a wait func. Because the
// handler acks on receipt and does download/verify/apply asynchronously,
// rover-side failures surface through this hook rather than the RPC reply.
// wait blocks until the hook fires (or fails the test after 2s).
func failureCatcher(t *testing.T) (hook func(string, map[string]string), wait func() (string, map[string]string)) {
	t.Helper()
	type res struct {
		msg string
		md  map[string]string
	}
	ch := make(chan res, 4)
	hook = func(msg string, md map[string]string) { ch <- res{msg, md} }
	wait = func() (string, map[string]string) {
		t.Helper()
		select {
		case r := <-ch:
			return r.msg, r.md
		case <-time.After(2 * time.Second):
			t.Fatal("OnApplyFailed did not fire within 2s")
			return "", nil
		}
	}
	return hook, wait
}

// installDoneWaiter wires Handler.onApplyDone so a test can block until the
// async apply goroutine returns (any failure, or a swupdate that exited
// without rebooting). Call it before Start so the hook is set before the
// goroutine reads it. The returned func waits with a generous backstop.
func installDoneWaiter(h *Handler) func(t *testing.T) {
	done := make(chan struct{}, 8)
	h.onApplyDone = func() { done <- struct{}{} }
	return func(t *testing.T) {
		t.Helper()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("apply goroutine did not finish within 10s")
		}
	}
}

// requestAccepted sends an apply request and asserts the immediate ack.
func requestAccepted(t *testing.T, h *Handler, req *waypointv1.ApplyImageRequest) {
	t.Helper()
	body, _ := proto.Marshal(req)
	rep, err := h.nc.Request("waypoint."+h.roverID+".rpc.apply_image", body, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var resp waypointv1.ApplyImageResponse
	if err := proto.Unmarshal(rep.Data, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "accepted" {
		t.Fatalf("status = %q (err=%q), want accepted", resp.Status, resp.GetError())
	}
}

func TestApplyImage_HappyPath(t *testing.T) {
	payload := make([]byte, 1024)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	want := sha256Hex(payload)

	srv := artifactServer(payload, []byte(`{"fake":"bundle"}`))
	defer srv.Close()

	nc, cleanup := natsTestConn(t)
	defer cleanup()

	cacheDir := t.TempDir()
	fv := &fakeVerifier{}
	h := &Handler{
		nc:          nc,
		roverID:     testRoverID,
		CacheDir:    cacheDir,
		SwupdateCmd: "/bin/true",
		HTTPClient:  http.DefaultClient,
		Verifier:    fv,
		applyDelay:  0,
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}

	expected := want
	req := &waypointv1.ApplyImageRequest{
		Url:            srv.URL + "/firmware.swu",
		ExpectedSha256: &expected,
	}
	waitDone := installDoneWaiter(h)
	requestAccepted(t, h, req)
	waitDone(t)

	if fv.calls.Load() != 1 {
		t.Fatalf("verifier called %d times, want 1", fv.calls.Load())
	}

	dst := filepath.Join(cacheDir, "incoming.swu")
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if sha256Hex(got) != want {
		t.Fatalf("downloaded sha mismatch: got %s want %s", sha256Hex(got), want)
	}

	// The cosign bundle should also be on disk next to the swu.
	if _, err := os.Stat(dst + ".cosign"); err != nil {
		t.Fatalf("cosign bundle missing: %v", err)
	}
}

func TestApplyImage_BadShaRejected(t *testing.T) {
	payload := []byte("hello swupdate")
	srv := artifactServer(payload, []byte(`{"fake":"bundle"}`))
	defer srv.Close()

	nc, cleanup := natsTestConn(t)
	defer cleanup()

	hook, waitFail := failureCatcher(t)
	fv := &fakeVerifier{}
	h := &Handler{
		nc:            nc,
		roverID:       testRoverID,
		CacheDir:      t.TempDir(),
		SwupdateCmd:   "/bin/true",
		HTTPClient:    http.DefaultClient,
		Verifier:      fv,
		applyDelay:    0,
		OnApplyFailed: hook,
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}

	wrong := "0000000000000000000000000000000000000000000000000000000000000000"
	req := &waypointv1.ApplyImageRequest{Url: srv.URL + "/firmware.swu", ExpectedSha256: &wrong}
	requestAccepted(t, h, req)

	msg, _ := waitFail()
	if !strings.Contains(msg, "sha mismatch") {
		t.Fatalf("failure %q does not mention sha mismatch", msg)
	}
	// sha check runs before verify, so verifier must not have been called.
	if fv.calls.Load() != 0 {
		t.Fatalf("verifier called on sha-mismatch path: %d times", fv.calls.Load())
	}
}

func TestApplyImage_MissingURL(t *testing.T) {
	nc, cleanup := natsTestConn(t)
	defer cleanup()

	fv := &fakeVerifier{}
	h := &Handler{
		nc:          nc,
		roverID:     testRoverID,
		CacheDir:    t.TempDir(),
		SwupdateCmd: "/bin/true",
		HTTPClient:  http.DefaultClient,
		Verifier:    fv,
		applyDelay:  0,
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}

	req := &waypointv1.ApplyImageRequest{}
	body, _ := proto.Marshal(req)
	rep, err := nc.Request("waypoint."+testRoverID+".rpc.apply_image", body, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var resp waypointv1.ApplyImageResponse
	if err := proto.Unmarshal(rep.Data, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "error" {
		t.Fatalf("status = %q, want error", resp.Status)
	}
	if resp.Error == nil || !strings.Contains(*resp.Error, "missing url") {
		t.Fatalf("error %q does not mention missing url", resp.GetError())
	}
}

func TestApplyImage_DownloadFails(t *testing.T) {
	// Start then immediately close an httptest server to get a known-closed URL.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := srv.URL
	srv.Close()

	nc, cleanup := natsTestConn(t)
	defer cleanup()

	hook, waitFail := failureCatcher(t)
	fv := &fakeVerifier{}
	h := &Handler{
		nc:            nc,
		roverID:       testRoverID,
		CacheDir:      t.TempDir(),
		SwupdateCmd:   "/bin/true",
		HTTPClient:    http.DefaultClient,
		Verifier:      fv,
		applyDelay:    0,
		OnApplyFailed: hook,
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}

	requestAccepted(t, h, &waypointv1.ApplyImageRequest{Url: deadURL + "/missing.swu"})

	msg, _ := waitFail()
	if !strings.Contains(msg, "download") {
		t.Fatalf("failure %q does not mention download", msg)
	}
}

// TestApplyImage_BundleDownloadFails covers the production case where the .swu
// URL resolves but the side-by-side .cosign bundle returns 404. swupdate must
// not be invoked and the failure must surface via OnApplyFailed. (Dev mode,
// which ships no bundle, is covered by TestApplyImage_DevModeSkipsBundle.)
func TestApplyImage_BundleDownloadFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".cosign") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	nc, cleanup := natsTestConn(t)
	defer cleanup()

	hook, waitFail := failureCatcher(t)
	fv := &fakeVerifier{}
	h := &Handler{
		nc:            nc,
		roverID:       testRoverID,
		CacheDir:      t.TempDir(),
		SwupdateCmd:   "/bin/true",
		HTTPClient:    http.DefaultClient,
		Verifier:      fv,
		applyDelay:    0,
		OnApplyFailed: hook,
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}

	requestAccepted(t, h, &waypointv1.ApplyImageRequest{Url: srv.URL + "/firmware.swu"})

	msg, _ := waitFail()
	if !strings.Contains(msg, "no cosign bundle at expected URL") {
		t.Fatalf("failure %q does not mention missing cosign bundle", msg)
	}
	if fv.calls.Load() != 0 {
		t.Fatalf("verifier called despite missing bundle: %d times", fv.calls.Load())
	}
}

// TestApplyImage_DevModeSkipsBundle proves that with WAYPOINT_SKIP_VERIFY=1
// (dev/QEMU images, which ship no .cosign sidecar) the apply proceeds without
// a bundle: the artifact server returns 404 for .cosign, yet swupdate still
// runs. This is the path the local dev OTA loop depends on.
func TestApplyImage_DevModeSkipsBundle(t *testing.T) {
	t.Setenv("WAYPOINT_SKIP_VERIFY", "1")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".cosign") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	nc, cleanup := natsTestConn(t)
	defer cleanup()

	argvPath := filepath.Join(t.TempDir(), "argv")
	cmdline, imgtoml := writeFixtures(t, "A", "dev")
	h := &Handler{
		nc:            nc,
		roverID:       testRoverID,
		CacheDir:      t.TempDir(),
		SwupdateCmd:   scriptRecordingArgs(t, argvPath),
		CmdlinePath:   cmdline,
		ImageTOMLPath: imgtoml,
		HTTPClient:    http.DefaultClient,
		Verifier:      &fakeVerifier{},
		applyDelay:    0,
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}

	argv := runApplyAndWaitForArgv(t, h, srv.URL+"/firmware.swu", argvPath)
	if len(argv) < 4 || argv[1] != "dev,copy-2" {
		t.Fatalf("swupdate argv = %v, want [-e dev,copy-2 -i <path>]", argv)
	}
}

// TestApplyImage_VerifyFails proves that a verifier rejection blocks
// swupdate. The reply must carry a "verify:" prefix and the swupdate binary
// must never be invoked. We assert "never invoked" by pointing SwupdateCmd
// at a script that would touch a sentinel, then asserting it isn't there
// after the reply.
func TestApplyImage_VerifyFails(t *testing.T) {
	srv := artifactServer([]byte("payload"), []byte(`{"fake":"bundle"}`))
	defer srv.Close()

	nc, cleanup := natsTestConn(t)
	defer cleanup()

	tmp := t.TempDir()
	sentinel := filepath.Join(tmp, "swupdate-ran")
	// A tiny shell script that writes the sentinel and exits 0. If swupdate
	// is invoked, this file appears; if not, the test passes.
	script := filepath.Join(tmp, "fake-swupdate.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+sentinel+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	hook, waitFail := failureCatcher(t)
	fv := &fakeVerifier{err: errors.New("cert SAN does not match pinned regex")}
	h := &Handler{
		nc:            nc,
		roverID:       testRoverID,
		CacheDir:      t.TempDir(),
		SwupdateCmd:   script,
		HTTPClient:    http.DefaultClient,
		Verifier:      fv,
		applyDelay:    0,
		OnApplyFailed: hook,
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}

	requestAccepted(t, h, &waypointv1.ApplyImageRequest{Url: srv.URL + "/firmware.swu"})

	msg, _ := waitFail()
	if !strings.HasPrefix(msg, "verify:") {
		t.Fatalf("failure %q does not start with verify:", msg)
	}
	if fv.calls.Load() != 1 {
		t.Fatalf("verifier called %d times, want 1", fv.calls.Load())
	}

	// Verify failed, so swupdate must never have been invoked.
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("swupdate was invoked even though verify failed")
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected stat err: %v", err)
	}
}

// TestApplyImage_SwupdateExitFailureIsHandled covers the case where verify
// passes and we reply "accepted", but swupdate itself fails (bad signature
// surfaced by swupdate, partition write error, disk full, …). The agent
// must not crash, the reply must still be "accepted" (it landed before
// swupdate ran), and a subsequent NATS request must still be handled —
// proving the goroutine logged the error and returned cleanly.
func TestApplyImage_SwupdateExitFailureIsHandled(t *testing.T) {
	srv := artifactServer([]byte("payload"), []byte(`{"fake":"bundle"}`))
	defer srv.Close()

	nc, cleanup := natsTestConn(t)
	defer cleanup()

	tmp := t.TempDir()
	sentinel := filepath.Join(tmp, "swupdate-ran")
	// Script touches the sentinel (proving swupdate was invoked) and then
	// exits 1, simulating swupdate failing before it could reboot.
	script := filepath.Join(tmp, "failing-swupdate.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+sentinel+"\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmdline, imgtoml := writeFixtures(t, "A", "dev")
	fv := &fakeVerifier{}
	h := &Handler{
		nc:            nc,
		roverID:       testRoverID,
		CacheDir:      t.TempDir(),
		SwupdateCmd:   script,
		CmdlinePath:   cmdline,
		ImageTOMLPath: imgtoml,
		HTTPClient:    http.DefaultClient,
		Verifier:      fv,
		applyDelay:    0,
	}
	waitDone := installDoneWaiter(h)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}

	req := &waypointv1.ApplyImageRequest{Url: srv.URL + "/firmware.swu"}
	requestAccepted(t, h, req)

	// Wait for the goroutine to invoke swupdate, observe its exit code, and
	// return; then the sentinel proves swupdate actually ran.
	waitDone(t)
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("swupdate was never invoked: %v", err)
	}

	// Agent must still be alive — prove it by sending a second request and
	// asserting a reply comes back within the timeout. If runSwupdate had
	// crashed the process or wedged the handler, this would time out.
	requestAccepted(t, h, req)
}

// scriptRecordingArgs returns the path to a shell script that writes its
// command-line arguments to argvPath (one per line) and exits 0. Used to
// assert swupdate was invoked with the expected `-e variant,copy-N -i path`.
func scriptRecordingArgs(t *testing.T, argvPath string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "record-argv.sh")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argvPath + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

// runApplyAndWaitForArgv issues an apply request, waits for the async apply
// goroutine to finish (the recording-script has run by then), and returns the
// recorded argv lines.
func runApplyAndWaitForArgv(t *testing.T, h *Handler, swuURL, argvPath string) []string {
	t.Helper()
	waitDone := installDoneWaiter(h)
	requestAccepted(t, h, &waypointv1.ApplyImageRequest{Url: swuURL})
	waitDone(t)
	data, err := os.ReadFile(argvPath)
	if err != nil || len(data) == 0 {
		t.Fatalf("swupdate script did not record argv: %v", err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

// TestApplyImage_SwupdateGetsCopy2WhenOnA pins the production-critical
// behavior we shipped in this commit: when booted on partition A, swupdate
// is invoked with `-e <variant>,copy-2 -i <path>`. Without this, swupdate
// can't pick a matching image set in sw-description and errors with
// "Compatible SW not found" — exactly the bug that bit the manual smoke
// before this code landed.
func TestApplyImage_SwupdateGetsCopy2WhenOnA(t *testing.T) {
	srv := artifactServer([]byte("payload"), []byte(`{"fake":"bundle"}`))
	defer srv.Close()

	nc, cleanup := natsTestConn(t)
	defer cleanup()

	argvPath := filepath.Join(t.TempDir(), "argv")
	cmdline, imgtoml := writeFixtures(t, "A", "dev")
	h := &Handler{
		nc:            nc,
		roverID:       testRoverID,
		CacheDir:      t.TempDir(),
		SwupdateCmd:   scriptRecordingArgs(t, argvPath),
		CmdlinePath:   cmdline,
		ImageTOMLPath: imgtoml,
		HTTPClient:    http.DefaultClient,
		Verifier:      &fakeVerifier{},
		applyDelay:    0,
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}

	argv := runApplyAndWaitForArgv(t, h, srv.URL+"/firmware.swu", argvPath)
	if len(argv) < 4 || argv[0] != "-e" || argv[1] != "dev,copy-2" || argv[2] != "-i" {
		t.Fatalf("swupdate argv = %v, want [-e dev,copy-2 -i <path>]", argv)
	}
}

// TestApplyImage_SwupdateGetsCopy1WhenOnB is the symmetric case: booted
// on rootfs-B, the apply targets copy-1 (which writes rootfs-A).
func TestApplyImage_SwupdateGetsCopy1WhenOnB(t *testing.T) {
	srv := artifactServer([]byte("payload"), []byte(`{"fake":"bundle"}`))
	defer srv.Close()

	nc, cleanup := natsTestConn(t)
	defer cleanup()

	argvPath := filepath.Join(t.TempDir(), "argv")
	cmdline, imgtoml := writeFixtures(t, "B", "prod")
	h := &Handler{
		nc:            nc,
		roverID:       testRoverID,
		CacheDir:      t.TempDir(),
		SwupdateCmd:   scriptRecordingArgs(t, argvPath),
		CmdlinePath:   cmdline,
		ImageTOMLPath: imgtoml,
		HTTPClient:    http.DefaultClient,
		Verifier:      &fakeVerifier{},
		applyDelay:    0,
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}

	argv := runApplyAndWaitForArgv(t, h, srv.URL+"/firmware.swu", argvPath)
	if len(argv) < 4 || argv[0] != "-e" || argv[1] != "prod,copy-1" || argv[2] != "-i" {
		t.Fatalf("swupdate argv = %v, want [-e prod,copy-1 -i <path>]", argv)
	}
}

// TestApplyImage_RefusesWithUnknownPartuuid covers the safety case: if
// the kernel cmdline has neither AAAA nor BBBB (e.g. boot from an unusual
// device, hand-edited cmdline, hardware mismatch), the agent MUST refuse
// to invoke swupdate rather than letting it apply to the wrong partition.
func TestApplyImage_RefusesWithUnknownPartuuid(t *testing.T) {
	srv := artifactServer([]byte("payload"), []byte(`{"fake":"bundle"}`))
	defer srv.Close()

	nc, cleanup := natsTestConn(t)
	defer cleanup()

	dir := t.TempDir()
	cmdline := filepath.Join(dir, "cmdline")
	if err := os.WriteFile(cmdline, []byte("root=PARTUUID=12345678-1234-1234-1234-123456789abc ro\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	imgtoml := filepath.Join(dir, "image.toml")
	if err := os.WriteFile(imgtoml, []byte("[image]\nvariant = \"dev\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	argvPath := filepath.Join(dir, "argv")
	h := &Handler{
		nc:            nc,
		roverID:       testRoverID,
		CacheDir:      t.TempDir(),
		SwupdateCmd:   scriptRecordingArgs(t, argvPath),
		CmdlinePath:   cmdline,
		ImageTOMLPath: imgtoml,
		HTTPClient:    http.DefaultClient,
		Verifier:      &fakeVerifier{},
		applyDelay:    0,
	}
	waitDone := installDoneWaiter(h)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}

	// Reply is still "accepted" (it lands before runSwupdate runs); the
	// refusal is in the apply goroutine and surfaces only via logs + the
	// absence of swupdate invocation.
	requestAccepted(t, h, &waypointv1.ApplyImageRequest{Url: srv.URL + "/firmware.swu"})

	// After the goroutine has run and returned, assert no argv was recorded.
	waitDone(t)
	if _, err := os.Stat(argvPath); err == nil {
		data, _ := os.ReadFile(argvPath)
		t.Fatalf("swupdate was invoked despite unknown PARTUUID: argv=%q", string(data))
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected stat err: %v", err)
	}
}

// TestApplyImage_OnApplyFailedFiresOnVerifyError proves the alerts hook
// runs on rover-side failure paths (so main.go can publish alert.raised
// without each error site needing to know about alerts). We use the verify
// path because it's the most representative of an actual image-apply fault
// — bad signature, expired cert, etc.
func TestApplyImage_OnApplyFailedFiresOnVerifyError(t *testing.T) {
	srv := artifactServer([]byte("payload"), []byte(`{"fake":"bundle"}`))
	defer srv.Close()

	nc, cleanup := natsTestConn(t)
	defer cleanup()

	var (
		mu     sync.Mutex
		gotMsg string
		gotMD  map[string]string
		calls  int
	)
	fv := &fakeVerifier{err: errors.New("cert SAN does not match pinned regex")}
	h := &Handler{
		nc:          nc,
		roverID:     testRoverID,
		CacheDir:    t.TempDir(),
		SwupdateCmd: "/bin/true",
		HTTPClient:  http.DefaultClient,
		Verifier:    fv,
		applyDelay:  0,
		OnApplyFailed: func(msg string, md map[string]string) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			gotMsg = msg
			gotMD = md
		},
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}

	url := srv.URL + "/firmware.swu"
	requestAccepted(t, h, &waypointv1.ApplyImageRequest{Url: url})

	// The hook fires on the async apply goroutine; wait for it.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := calls > 0
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("OnApplyFailed called %d times, want 1", calls)
	}
	if !strings.HasPrefix(gotMsg, "verify:") {
		t.Errorf("OnApplyFailed message = %q, want prefix \"verify:\"", gotMsg)
	}
	if gotMD["url"] != url {
		t.Errorf("OnApplyFailed metadata[url] = %q, want %q", gotMD["url"], url)
	}
}

// TestApplyImage_OnApplyFailedDoesNotFireOnClientErrors documents the
// deliberate carve-out: "bad request" and "missing url" are caller bugs,
// not rover faults, and would flood the alerts store on malformed RPCs.
func TestApplyImage_OnApplyFailedDoesNotFireOnClientErrors(t *testing.T) {
	nc, cleanup := natsTestConn(t)
	defer cleanup()

	var calls atomic.Int32
	h := &Handler{
		nc:          nc,
		roverID:     testRoverID,
		CacheDir:    t.TempDir(),
		SwupdateCmd: "/bin/true",
		HTTPClient:  http.DefaultClient,
		Verifier:    &fakeVerifier{},
		applyDelay:  0,
		OnApplyFailed: func(string, map[string]string) {
			calls.Add(1)
		},
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}

	// Missing url → client error.
	body, _ := proto.Marshal(&waypointv1.ApplyImageRequest{})
	if _, err := nc.Request("waypoint."+testRoverID+".rpc.apply_image", body, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	// Malformed proto → client error (not a real ApplyImageRequest).
	if _, err := nc.Request("waypoint."+testRoverID+".rpc.apply_image", []byte("\xff\xff\xff"), 2*time.Second); err != nil {
		t.Fatal(err)
	}

	// Give the handler a moment in case a wayward hook is racing.
	time.Sleep(100 * time.Millisecond)
	if got := calls.Load(); got != 0 {
		t.Fatalf("OnApplyFailed fired on client errors: %d calls", got)
	}
}

// TestApplyImage_RefusesWhenImageTOMLMissing covers the symmetric safety
// case: PARTUUID is recognizable but image.toml is missing or unreadable.
// We refuse rather than guessing a variant.
func TestApplyImage_RefusesWhenImageTOMLMissing(t *testing.T) {
	srv := artifactServer([]byte("payload"), []byte(`{"fake":"bundle"}`))
	defer srv.Close()

	nc, cleanup := natsTestConn(t)
	defer cleanup()

	dir := t.TempDir()
	cmdline := filepath.Join(dir, "cmdline")
	if err := os.WriteFile(cmdline, []byte("root=PARTUUID=aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa ro\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	missingImgTOML := filepath.Join(dir, "does-not-exist.toml")

	argvPath := filepath.Join(dir, "argv")
	h := &Handler{
		nc:            nc,
		roverID:       testRoverID,
		CacheDir:      t.TempDir(),
		SwupdateCmd:   scriptRecordingArgs(t, argvPath),
		CmdlinePath:   cmdline,
		ImageTOMLPath: missingImgTOML,
		HTTPClient:    http.DefaultClient,
		Verifier:      &fakeVerifier{},
		applyDelay:    0,
	}
	waitDone := installDoneWaiter(h)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}

	requestAccepted(t, h, &waypointv1.ApplyImageRequest{Url: srv.URL + "/firmware.swu"})

	waitDone(t)
	if _, err := os.Stat(argvPath); err == nil {
		t.Fatal("swupdate was invoked despite missing image.toml")
	}
}

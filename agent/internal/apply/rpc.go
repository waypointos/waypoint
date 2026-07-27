// Package apply handles rpc.apply_image: download a .swu artifact and hand
// it to swupdate.
package apply

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/waypointos/waypoint/agent/internal/partition"
	"github.com/waypointos/waypoint/agent/internal/verify"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// Handler subscribes to waypoint.<rover>.rpc.apply_image, downloads the
// referenced .swu artifact and its cosign bundle, verifies the bundle against
// the pinned image-build OIDC identity, optionally verifies the artifact
// sha256, replies "accepted" to the caller, and then invokes swupdate.
//
// The reply is intentionally sent BEFORE swupdate runs because swupdate
// triggers a reboot. Verification, however, runs BEFORE the reply: if the
// bundle fails to verify, swupdate is never invoked and the caller gets an
// error reply.
type Handler struct {
	nc      *nats.Conn
	roverID string

	// CacheDir is where the downloaded .swu (and its .cosign bundle) are
	// written. Defaults to /run/waypoint/cache, a tmpfs: the artifact is
	// tens of MB and the read-only /data partition is too small to hold it.
	// Override in tests with a t.TempDir().
	CacheDir string

	// SwupdateCmd is the binary invoked after the reply lands. Defaults to
	// /usr/bin/swupdate; tests override to /bin/true.
	SwupdateCmd string

	// CmdlinePath is the path to the kernel cmdline. Read to determine the
	// currently-running partition (rootfs-A vs rootfs-B). Defaults to
	// /proc/cmdline; tests inject a fixture.
	CmdlinePath string

	// ImageTOMLPath is the path to the build-time image metadata file. Read
	// to determine the variant ("dev" or "prod") for the swupdate selector.
	// Defaults to /etc/waypoint/image.toml; tests inject a fixture.
	ImageTOMLPath string

	// HTTPClient downloads .swu and .cosign URLs. Defaults to http.DefaultClient.
	HTTPClient *http.Client

	// Verifier validates the .swu against the .cosign bundle. Defaults to
	// the production Sigstore verifier; tests inject a fake.
	Verifier verify.Verifier

	// applyDelay is a small settling pause before invoking swupdate. Tests
	// set this to 0.
	applyDelay time.Duration

	// OnApplyFailed, when non-nil, is called after each rover-side failure
	// (download / sha256 / verify). Because the caller is acked on receipt,
	// this hook is the only failure signal for those paths: main.go wires it
	// to alerts.PublishRaised with code=image_apply_failed. We intentionally
	// do NOT call it on client errors like "bad request" or "missing url" —
	// those are caller bugs, not rover faults, and would flood the alerts
	// store on malformed RPCs.
	//
	// Implementations should be cheap and non-blocking; the hook runs on the
	// apply goroutine.
	OnApplyFailed func(message string, metadata map[string]string)

	// onApplyDone, when non-nil, is invoked when the async apply goroutine
	// returns (any failure, or a swupdate that exited without rebooting).
	// Test-only seam to await the goroutine deterministically; it never fires
	// on the success path because swupdate reboots before returning.
	onApplyDone func()
}

// NewHandler returns a Handler with production defaults.
func NewHandler(nc *nats.Conn, roverID string) *Handler {
	return &Handler{
		nc:            nc,
		roverID:       roverID,
		CacheDir:      "/run/waypoint/cache",
		SwupdateCmd:   "/usr/bin/swupdate",
		CmdlinePath:   "/proc/cmdline",
		ImageTOMLPath: "/etc/waypoint/image.toml",
		HTTPClient:    http.DefaultClient,
		Verifier:      verify.NewSigstore(),
		applyDelay:    500 * time.Millisecond,
	}
}

// Start subscribes to the apply_image RPC subject.
func (h *Handler) Start() error {
	subj := fmt.Sprintf("waypoint.%s.rpc.apply_image", h.roverID)
	_, err := h.nc.Subscribe(subj, h.onMessage)
	return err
}

func (h *Handler) onMessage(msg *nats.Msg) {
	var req waypointv1.ApplyImageRequest
	if err := proto.Unmarshal(msg.Data, &req); err != nil {
		// Client error — don't fire OnApplyFailed; see Handler.OnApplyFailed.
		h.reply(msg, errorResp("bad request: "+err.Error()))
		return
	}
	if req.GetUrl() == "" {
		h.reply(msg, errorResp("missing url"))
		return
	}

	// Acknowledge on receipt, then do the slow work (download → verify →
	// swupdate) in the background. The .swu is tens of MB, so download alone
	// can exceed the caller's RPC timeout, and swupdate reboots the rover —
	// a reply sent after that work would never arrive. The terminal outcome
	// is reported out-of-band: success via the bumped infra.system.image
	// version, failure via OnApplyFailed (alert.raised). "accepted" therefore
	// means "request received, applying", not "verified".
	h.reply(msg, &waypointv1.ApplyImageResponse{Status: "accepted"})
	go h.fetchVerifyApply(&req)
}

// fetchVerifyApply downloads the artifact (and, outside dev mode, its cosign
// bundle), verifies it, and invokes swupdate. It runs in its own goroutine
// after the caller has already been acked, so failures surface only through
// OnApplyFailed (alert.raised) and logs, not an RPC reply.
func (h *Handler) fetchVerifyApply(req *waypointv1.ApplyImageRequest) {
	if h.onApplyDone != nil {
		defer h.onApplyDone()
	}
	// metadata captures the request fields useful for alert triage; we keep
	// it small (url only) — the dashboard cross-references the rover-id from
	// the subject, and the version isn't always set by callers.
	metadata := map[string]string{"url": req.GetUrl()}
	fail := func(reason string) {
		slog.Warn("apply: " + reason)
		if h.OnApplyFailed != nil {
			h.OnApplyFailed(reason, metadata)
		}
	}

	if err := os.MkdirAll(h.CacheDir, 0o750); err != nil {
		fail("cache: " + err.Error())
		return
	}
	dst := filepath.Join(h.CacheDir, "incoming.swu")
	bundleDst := dst + ".cosign"

	if err := h.download(req.GetUrl(), dst); err != nil {
		fail("download: " + err.Error())
		return
	}

	// Dev images skip signature verification (WAYPOINT_SKIP_VERIFY=1) and ship
	// no .cosign sidecar, so don't require one. Production always does: the CI
	// workflow uploads <name>.swu and <name>.swu.cosign side-by-side, and the
	// bundle URL is derived by convention to avoid a protocol change.
	if !verify.SkipEnabled() {
		if err := h.download(req.GetUrl()+".cosign", bundleDst); err != nil {
			fail("no cosign bundle at expected URL: " + err.Error())
			return
		}
	}

	if want := req.GetExpectedSha256(); want != "" {
		sum, err := sha256File(dst)
		if err != nil {
			fail("sha256: " + err.Error())
			return
		}
		if sum != want {
			fail(fmt.Sprintf("sha mismatch: got %s want %s", sum, want))
			return
		}
	}

	if err := h.Verifier.Verify(context.Background(), dst, bundleDst); err != nil {
		fail("verify: " + err.Error())
		return
	}

	if err := h.runSwupdate(dst, req.GetVersion()); err != nil {
		fail(err.Error())
	}
}

// runSwupdate invokes swupdate against the freshly-downloaded .swu. On
// success swupdate reboots the rover, so this function does not return. On
// failure (bad selector, partition-write error, disk full, ...) it returns a
// non-nil error so the caller can raise an alert; the rover stays on the
// current partition and the next infra.system.image publish still reports the
// old version, a second structural failure signal.
//
// The `-e <variant>,copy-N` selector tells swupdate which of the two
// "copy-1 / copy-2" image sets in sw-description to apply: copy-2 when
// running on A (writes B), copy-1 when running on B (writes A). Without
// the selector, swupdate looks for top-level images and reports
// "Compatible SW not found".
func (h *Handler) runSwupdate(dst, version string) error {
	time.Sleep(h.applyDelay)

	selector, err := h.swupdateSelector()
	if err != nil {
		// Refusing to invoke swupdate without -e (it would fail with
		// "Compatible SW not found"); stay on the current partition.
		return fmt.Errorf("swupdate selector (intended version=%q): %w", version, err)
	}

	cmd := exec.CommandContext(context.Background(), h.SwupdateCmd, "-e", selector, "-i", dst)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("swupdate failed (selector=%s, intended version=%q): %w", selector, version, err)
	}
	// swupdate-on-success would have triggered reboot before we reach here.
	// Reaching this line means swupdate exited 0 without rebooting, which is
	// itself surprising and worth flagging.
	slog.Warn(fmt.Sprintf("apply: swupdate exited 0 without rebooting for %s (selector=%s, intended version=%q)",
		dst, selector, version))
	return nil
}

// swupdateSelector builds the "variant,copy-N" string passed to
// `swupdate -e`. It reads the currently-running partition from the
// kernel cmdline and the variant from /etc/waypoint/image.toml; both
// must be present for the apply to proceed.
func (h *Handler) swupdateSelector() (string, error) {
	active, err := partition.Active(h.CmdlinePath)
	if err != nil {
		return "", fmt.Errorf("active partition: %w", err)
	}
	variant, err := readVariant(h.ImageTOMLPath)
	if err != nil {
		return "", fmt.Errorf("variant: %w", err)
	}
	target := "copy-2" // booted on A → write to B
	if active == "B" {
		target = "copy-1" // booted on B → write to A
	}
	return variant + "," + target, nil
}

// readVariant pulls image.variant out of /etc/waypoint/image.toml. The
// file is written at image build time by post-build.sh.
func readVariant(path string) (string, error) {
	var t struct {
		Image struct {
			Variant string `toml:"variant"`
		} `toml:"image"`
	}
	if _, err := toml.DecodeFile(path, &t); err != nil {
		return "", err
	}
	if t.Image.Variant == "" {
		return "", errors.New("image.toml: image.variant is empty")
	}
	return t.Image.Variant, nil
}

func (h *Handler) reply(msg *nats.Msg, resp *waypointv1.ApplyImageResponse) {
	if msg.Reply == "" {
		return
	}
	body, _ := proto.Marshal(resp)
	_ = h.nc.Publish(msg.Reply, body)
}

func errorResp(s string) *waypointv1.ApplyImageResponse {
	return &waypointv1.ApplyImageResponse{Status: "error", Error: &s}
}

func (h *Handler) download(url, dst string) error {
	resp, err := h.HTTPClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

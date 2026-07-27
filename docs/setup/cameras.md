# Cameras

The agent owns every camera and streams it to the dashboard over WebRTC (WHEP).
It supervises each pipeline and restarts it on failure.

## Auto-discovery

By default the agent discovers cameras itself. At boot it enumerates `/dev/video*`,
keeps the nodes that expose a video-capture format (skipping the metadata nodes
UVC webcams present), and starts one pipeline per device. It re-scans every 30
seconds, so plugging or unplugging a USB camera is reflected live. Discovered
cameras are named `camera-0`, `camera-1`, and so on, after the `/dev/videoN` index.

No camera configuration is required for the common case: leave `[[cameras]]` out
of `identity.toml` and the agent finds the hardware on its own.

## Pinning cameras explicitly (optional)

Declare `[[cameras]]` in `identity.toml` only when you want stable names, a
specific device, or a forced pipeline. Explicit cameras are honored verbatim and
are not hot-swapped; declaring even one camera turns auto-discovery off.

```toml
[[cameras]]
name     = "chassis-front"
device   = "/dev/video0"     # on macOS dev: "0" (avf index)
pipeline = "pi5"             # synthetic | mac | pi5; omit to auto-detect by OS

[[cameras]]
name     = "gripper"
device   = "/dev/video2"
pipeline = "pi5"
```

On macOS with no webcam, set `pipeline = "synthetic"` to drive the dashboard with
a test pattern.

## GStreamer

macOS:

```
brew install gstreamer gst-plugins-base gst-plugins-good gst-plugins-bad gst-plugins-ugly
```

```
gst-launch-1.0 --version
gst-inspect-1.0 avfvideosrc
```

Pi 5 (Debian Bookworm):

```
sudo apt install gstreamer1.0-tools gstreamer1.0-plugins-base gstreamer1.0-plugins-good gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly gstreamer1.0-libav
```

The Pi 5 has no hardware H.264 encoder, so the `pi5` pipeline captures MJPEG and
encodes H.264 in software with `x264enc` (`v4l2src ! jpegdec ! videoconvert !
x264enc ! h264parse`). That element ships in `gstreamer1.0-plugins-ugly`; verify
it is present:

```
gst-inspect-1.0 x264enc
```

## TURN

Camera media is relayed through TURN when a direct browser-to-rover path is not
available. Cloudflare Realtime TURN is preferred. Generate a TURN API key, then
set it on the proxy (the agent fetches the config from the proxy at boot) and,
optionally, on the agent directly:

```
CLOUDFLARE_TURN_KEY=<turn-key-id>:<turn-key-token>
```

When unset, the agent advertises only host candidates (LAN-direct only).

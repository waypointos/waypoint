//go:build !(linux && arm64)

package cameras

// discoverCameras returns nothing off-rover; auto-discovery is a Pi 5 (V4L2)
// concern. Non-rover builds rely on explicit identity.toml cameras (or none).
func discoverCameras() []CameraSpec { return nil }

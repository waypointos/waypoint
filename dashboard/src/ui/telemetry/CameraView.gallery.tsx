// dashboard/src/ui/telemetry/CameraView.gallery.tsx
//
// Gallery example for CameraView. Points at the local agent's synthetic
// pipeline; see docs/setup/cameras.md to enable that pipeline in
// identity.toml. With the agent running (`cd agent && go run
// ./cmd/waypoint-agent`), the tile shows a moving test pattern.
import { CameraView } from './CameraView';

export function CameraViewGalleryExample() {
  return (
    <div style={{ width: 480 }}>
      <CameraView
        whepUrl="/camera/chassis-front/whep"
        label="chassis-front"
      />
    </div>
  );
}

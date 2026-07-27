# `dashboard/src/ui/` — design system

Source of truth for visual components, tokens, and patterns. Never write a UI
element from scratch without checking this directory first.

## Layout

- `tokens.css` — CSS custom properties. The ONLY way colors and spacing enter
  the rest of the code.
- `tokens.ts`  — same tokens exported for JS/TS (charts, inline SVG, hooks).
- `primitives/` — generic building blocks (Button, Panel, Chip, …).
- `data/`       — telemetry-shaped components (Metric, StatusDot, …).
- `telemetry/`  — rover-specific compound components (JoystickPad, MotorCard…).
- `nav/`        — Sidebar, TopBar, Crumbs, OperatorClock.
- `feedback/`   — Toast, Banner.
- `__gallery__/Gallery.tsx` — rendered at `/ui-gallery`; every component in
  every state. Replaces Storybook.

## Rules

1. NEVER use hex colors inline. Always tokens.
2. NEVER use Tailwind / MUI / Chakra / styled-components.
3. Icons come from `lucide-react`. No icon fonts.
4. Mono is the default for chrome (labels, panel titles, eyebrows, numbers).
   Sans is the default for body content.
5. All-caps is reserved for very small eyebrow labels (≤11 px), HUD overlays,
   and the E-Stop button.
6. Every nullable telemetry field renders muted `N/A` + a one-line reason hint.
7. New primitive? Add to `primitives/` and update `__gallery__/Gallery.tsx`.

## Module panels

Module-contributed panels live with their module (e.g.,
`modules/umr/dashboard/src/ConnectionPanel.tsx`). The host gallery does not
import them; visit the running dashboard with the relevant module enabled
to see them in context.

# Basemap tile bucket (proxy)

The proxy serves the dashboard map tiles from a self-hosted PMTiles file in an
S3-compatible bucket (a Railway Bucket, backed by Tigris), gated behind the
session cookie so the endpoint isn't an open tile proxy. The rover caches the
tiles it views (over NATS) so it keeps working offline. Until the bucket has a
tileset, the map renders blank (no crash; the proxy opens the bucket lazily).

## 1. Connect the bucket to the proxy
1. Railway → create a **Bucket** (or use an existing one).
2. On the bucket's **Credentials** tab → **Add to Service** → pick `waypoint-proxy`,
   Style **AWS SDK (Generic)**. This injects these env vars into the proxy:
   `AWS_ENDPOINT_URL`, `AWS_S3_BUCKET_NAME`, `AWS_DEFAULT_REGION`,
   `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`.

The proxy auto-detects `AWS_S3_BUCKET_NAME` and reads tiles from `s3://<bucket>`,
taking the endpoint/region/credentials from those `AWS_*` vars (virtual-hosted
addressing, which Tigris requires; this is the AWS SDK v2 default, handled in
`proxy/internal/basemap/server.go`). No volume.

Override with `WAYPOINT_PROXY_BASEMAP_URL` (e.g. a specific `s3://…` or a local
dir for dev) if you don't want the auto-derived bucket.

## 2. Build the tileset (local, no 120 GB download)
1. Install the `pmtiles` CLI: `go install github.com/protomaps/go-pmtiles@latest`.
2. Get the current planet build URL from https://docs.protomaps.com/basemaps/downloads.
   The Source Cooperative mirror is a common source:
   `https://data.source.coop/protomaps/openstreetmap/v4.pmtiles`
3. Run the seed (`env` form for fish):
   ```fish
   env SRC="https://data.source.coop/protomaps/openstreetmap/v4.pmtiles" scripts/basemap/seed.sh
   ```
   → `scripts/basemap/basemap.pmtiles` (world z0–7 + the `regions.geojson` areas z8–15).
   Edit `scripts/basemap/regions.geojson` to change coverage.

## 3. Upload to the bucket
The object **must** be named `basemap.pmtiles` at the bucket root; the server
maps `/basemap/{z}/{x}/{y}.mvt` to the `basemap` archive by filename stem.
Use the bucket's S3 credentials with any S3 client (Railway's UI, `rclone`, or
the AWS CLI). AWS CLI example (Tigris wants virtual-hosted addressing):
```fish
aws configure set default.s3.addressing_style virtual
env AWS_ACCESS_KEY_ID=… AWS_SECRET_ACCESS_KEY=… AWS_DEFAULT_REGION=auto \
  aws --endpoint-url https://t3.storageapi.dev \
  s3 cp scripts/basemap/basemap.pmtiles s3://<bucket-name>/basemap.pmtiles
```
The proxy opens the bucket lazily, so the next tile request picks it up; a
restart isn't strictly required.

## 4. Deploy note: rebuild the embedded dashboard
The map's tile source and vendored glyphs (`/mapfonts/`) ship inside the embedded
dashboard build. After pulling this change, rebuild + embed the dashboard
(`make embed-dashboard`, or the usual dashboard build step) before deploying the
proxy and flashing the agent; otherwise the served dist predates `mapfonts/`
and labels 404.

## Refreshing coverage
Re-run the seed with an updated `SRC` or edited `regions.geojson`, re-upload,
restart. Rovers' on-device caches (read-through, over NATS) refill naturally as
they re-request tiles.

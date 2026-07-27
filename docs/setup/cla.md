# Setup: CLA enforcement

The [`.github/workflows/cla.yml`](../../.github/workflows/cla.yml) workflow gates
pull requests on a signed [Contributor License Agreement](../../CLA.md), using
[CLA Assistant Lite](https://github.com/contributor-assistant/github-action).
Contributors sign by commenting on their PR; signatures are stored as JSON in
the repository.

## One-time maintainer setup

No tokens or secrets to create. Because signatures are stored in this same
repository, the built-in `GITHUB_TOKEN` does everything. Two steps:

1. **Allow Actions to write and comment on PRs:** repo Settings > Actions >
   General > Workflow permissions > enable "Read and write permissions" and
   "Allow GitHub Actions to create and approve pull requests."

2. **Push `CLA.md` to the default branch** so the `path-to-document` URL in the
   workflow resolves. (It points at `main`; the relicense commit handles this.)

That's it. The first pull request after this will exercise the bot.

## How it works

- On each PR, the workflow checks whether every contributor has signed.
- Unsigned contributors are asked to comment the exact phrase:
  `I have read the CLA Document and I hereby sign the CLA`
- Signatures are recorded in `signatures/cla.json` on a `cla-signatures` branch.
  This branch is created automatically on the first signature.
- The repo owner and bots are allow-listed and skip signing (see `allowlist` in
  the workflow).
- A contributor signs once; later PRs are recognized automatically. Commenting
  `recheck` re-runs the check.

## Notes

- The CLA text lives in [`CLA.md`](../../CLA.md). If you change it materially,
  contributors who already signed the old version should re-sign; clearing
  `signatures/cla.json` forces everyone to sign again.
- `CLA.md` is a template. Have a lawyer review it before relying on it for a
  commercial relicense.

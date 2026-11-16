# 1. Maintain a hardened build of Gotenberg to replace opg-pdf-service

Date: 2026-11-16

## Status

Accepted

## Context

HTML to PDF conversion for Sirius was provided by [opg-pdf-service](https://github.com/ministryofjustice/opg-pdf-service), a Node.js/Puppeteer web server exposing `/generate-pdf`. The service was no longer working and carried an ongoing maintenance burden.

We replaced it with [Gotenberg](https://github.com/gotenberg/gotenberg), an actively developed, widely used, API-driven conversion service that wraps Chromium, which removes the need for us to maintain conversion logic ourselves.

Once Gotenberg was in place we found the latest images contained many known vulnerabilities and had a slow release cycle, so fixes for those vulnerabilities, including those already fixed in dependencies, can take a long time to reach a published image.

We considered three options:

1. **Rewrite the PDF service in Go.** Full control, but re-creates the maintenance burden we were trying to remove and duplicates what Gotenberg already does.
2. **Use WeasyPrint.** Smaller attack surface than a full browser, but it is not a browser engine. Rendering differs from Chromium, so existing Sirius documents would need to be reworked and re-validated.
3. **Rebuild and publish our own version of Gotenberg.** Keeps the upstream API and Chromium rendering, and lets us apply security patches ourselves while still tracking upstream.

## Decision

We will build, test and publish our own hardened image of Gotenberg from this repository (option 3), published to ECR as `shared/opg-gotenberg`.

The image is based on the upstream `gotenberg/gotenberg` image and is patched in the following ways:

- **Chromium-only image.** Based on the `-chromium` variant, as we only use HTML conversion and stamping. This drops LibreOffice and its dependencies from the attack surface.
- **Pinned base.** The base image is pinned by tag and digest. Renovate raises updates, with `minimumReleaseAge` of 7 days for normal updates and immediate PRs for vulnerability alerts.
- **OS package patching.** An `apt-patch` stage upgrades only the affected packages and copies the result into the final image. `ca-certificates` was bumped specifically to pick up a release with compromised CAs removed.
- **`pdftk` jar patching.** A `jar-patch` stage replaces the bundled `commons-lang3` in `pdftk-all.jar` with a checksum-verified newer release.
- **Rebuilding Go binaries.** A `go-patch` stage builds `gotenberg-chromium` from a pinned upstream commit with a newer Go toolchain and with vulnerable dependencies bumped to fixed commits, followed by `go mod verify`. `pdfcpu` is likewise rebuilt from a pinned commit. Each of these stages should be removed once upstream ships the fix.
- **Hardened runtime.** The container runs as the non-root `gotenberg` user, is not privileged, and uses a read-only root filesystem with sized `tmpfs` mounts. CI verifies that `docker-compose.yml` matches the ECS mount configuration in SSM.
- **Snyk ignores.** Where there's no fix available yet, the finding goes in `.snyk` with a reason and an expiry date, so it can be regularly reviewed.

Because we diverge from upstream, we also own the verification that the image still works:

- **Goss tests** (`test/goss.yaml`) check the health endpoint and a basic HTML to PDF conversion against the running container.
- **Visual regression tests** (`test/visual`) convert Sirius documents, render pages with `pdftoppm` and compare them with committed baseline images using pixel matching. Baselines are updated deliberately with `make update-baselines`.

## Consequences

- We stay compatible with the upstream Gotenberg API and Chromium rendering, so the migration away from opg-pdf-service does not need document rework.
- We can fix vulnerabilities on our own timeline rather than upstream's.
- We take on responsibility for the patched layers: the `Dockerfile` pins (commits, versions, checksums) must be reviewed on each Gotenberg upgrade, and patch stages removed once upstream includes the fixes.
- Some vulnerabilities have no fix available (for example Debian packages) and can only be mitigated by exceptions in `.snyk`. These expire and need regular review.
- The visual baselines can produce false positives when Chromium or fonts change and require a deliberate review before updating.

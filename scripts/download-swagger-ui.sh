#!/usr/bin/env bash
#
# Downloads Swagger UI dist files and vendors them for go:embed.
#
set -euo pipefail

VERSION="${1:-5.17.14}"
TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

echo "Downloading Swagger UI v${VERSION}..."
curl -sL --insecure "https://github.com/swagger-api/swagger-ui/archive/refs/tags/v${VERSION}.tar.gz" -o "$TMPDIR/swagger-ui.tar.gz"
tar -xzf "$TMPDIR/swagger-ui.tar.gz" -C "$TMPDIR"

DIST="$TMPDIR/swagger-ui-${VERSION}/dist"

for TARGET in internal/rpc/swagger-ui internal/indexer/swagger-ui; do
    echo "Vendoring to $TARGET/"
    cp "$DIST/swagger-ui.css" "$TARGET/"
    cp "$DIST/swagger-ui-bundle.js" "$TARGET/"
    cp "$DIST/swagger-ui-standalone-preset.js" "$TARGET/"
    cp "$DIST/favicon-32x32.png" "$TARGET/" 2>/dev/null || true
done

echo "Done. Swagger UI v${VERSION} vendored."
echo "Run 'go build ./...' to embed the assets."

#!/usr/bin/env bash
# Upload the extensions directory to Artifactory.

set -Eeuo pipefail

cd ./extensions
find . -type f -exec curl -H "X-JFrog-Art-Api:$ARTIFACTORY_TOKEN" -T '{}' "$ARTIFACTORY_URI/$ARTIFACTORY_REPO/"'{}' \;

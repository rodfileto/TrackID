#!/usr/bin/env bash
set -euo pipefail

# Download ROBOT (robot.jar) into backend/tools/ for the ontology reasoner
# pass (see app/ontology/reasoner.py). Requires Java at runtime; robot.jar is
# gitignored.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "$DIR/tools"
curl -sSL -o "$DIR/tools/robot.jar" \
  https://github.com/ontodev/robot/releases/latest/download/robot.jar
echo "Downloaded robot.jar -> $DIR/tools/robot.jar"

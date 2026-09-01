#!/usr/bin/env bash
set -euo pipefail

# Self-test for the ontology verification tooling (docs/ONTOLOGY_BFO.md).
#
# Writes a temporary `tid:` domain ontology into vendor/tid/, asserts that both
# checks pass (conformance + DL-reasoner consistency), then breaks one class
# (makes it both a continuant and an occurrent) and asserts both checks fail.
#
# Requires Java + robot.jar (scripts/fetch_robot.sh). Takes a few minutes: the
# reasoner merge over BFO + all of CCO is ~100s, run twice (good + bad).
#
# Usage: scripts/ontology_self_test.sh   (run from anywhere)

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TID_DIR="$DIR/app/ontology/vendor/tid"
PYTHON="${PYTHON:-$DIR/venv/bin/python}"
ROBOT_JAR="${ROBOT_JAR:-$DIR/tools/robot.jar}"

if [ ! -f "$ROBOT_JAR" ]; then
  echo "robot.jar not found at $ROBOT_JAR; run scripts/fetch_robot.sh first." >&2
  exit 2
fi
if [ -e "$TID_DIR" ]; then
  echo "Refusing to run: $TID_DIR already exists (move it aside first)." >&2
  exit 2
fi

cleanup() { rm -rf "$TID_DIR"; }
trap cleanup EXIT

write_tid() {
  # $1 = "good" | "bad"; "bad" adds `process` as a second parent, which puts
  # EnrollmentFaceImage under both a continuant and an occurrent.
  local extra=""
  [ "$1" = "bad" ] && extra=" , obo:BFO_0000015"
  mkdir -p "$TID_DIR"
  cat > "$TID_DIR/face-record.ttl" <<EOF
@prefix obo:  <http://purl.obolibrary.org/obo/> .
@prefix cco:  <https://www.commoncoreontologies.org/> .
@prefix tid:  <https://trackid.example.org/ontology/> .
@prefix owl:  <http://www.w3.org/2002/07/owl#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .

tid:Face a owl:Class ;
    rdfs:subClassOf obo:BFO_0000024 ;
    rdfs:label "Face"@en .

tid:FaceImage a owl:Class ;
    rdfs:subClassOf cco:ont00002004 ;
    rdfs:label "Face Image"@en .

tid:FaceEmbedding a owl:Class ;
    rdfs:subClassOf cco:ont00001163 ;
    rdfs:label "Face Embedding"@en .

tid:FaceRecord a owl:Class ;
    rdfs:subClassOf obo:BFO_0000031 ;
    rdfs:label "Face Record"@en .

tid:SurveillanceFaceImage a owl:Class ;
    rdfs:subClassOf tid:FaceImage ;
    rdfs:label "Surveillance Face Image"@en .

tid:EnrollmentFaceImage a owl:Class ;
    rdfs:subClassOf tid:FaceImage${extra} ;
    rdfs:label "Enrollment Face Image"@en .

tid:IdentityDocument a owl:Class ;
    rdfs:subClassOf cco:ont00001346 ;
    rdfs:label "Identity Document"@en .

tid:Passport a owl:Class ;
    rdfs:subClassOf tid:IdentityDocument ;
    rdfs:label "Passport"@en .

tid:ActOfSurveillance a owl:Class ;
    rdfs:subClassOf obo:BFO_0000015 ;
    rdfs:label "Act of Surveillance"@en .

tid:Enrollment a owl:Class ;
    rdfs:subClassOf obo:BFO_0000015 ;
    rdfs:label "Enrollment"@en .
EOF
}

cd "$DIR"

echo "== Phase 1: correct domain ontology =="
write_tid good
"$PYTHON" -m app.ontology.validation
"$PYTHON" -m app.ontology.reasoner

echo
echo "== Phase 2: EnrollmentFaceImage also a process (bug) =="
write_tid bad
if "$PYTHON" -m app.ontology.validation; then
  echo "FAIL: conformance check should have failed" >&2
  exit 1
fi
if "$PYTHON" -m app.ontology.reasoner; then
  echo "FAIL: reasoner should have failed" >&2
  exit 1
fi

echo
echo "Self-test passed."

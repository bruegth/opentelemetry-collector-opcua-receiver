#!/bin/bash
# Validates the OTel collector file export against expected test records.
# Usage: ./validate.sh <output-file> <expected-file>
#
# The output file contains OTLP JSON log export lines.
# The expected file contains the array of expected records with known fields.

set -euo pipefail

OUTPUT_FILE="${1:-/output/logs.json}"
EXPECTED_FILE="${2:-/expected/records.json}"
MAX_WAIT="${3:-60}"

echo "=== OPC UA E2E Test Validation ==="
echo "Output file:   $OUTPUT_FILE"
echo "Expected file: $EXPECTED_FILE"

# Wait for the output file to exist and have content
elapsed=0
while [ ! -s "$OUTPUT_FILE" ] && [ "$elapsed" -lt "$MAX_WAIT" ]; do
    echo "Waiting for output file... (${elapsed}s/${MAX_WAIT}s)"
    sleep 5
    elapsed=$((elapsed + 5))
done

if [ ! -s "$OUTPUT_FILE" ]; then
    echo "FAIL: Output file $OUTPUT_FILE does not exist or is empty after ${MAX_WAIT}s"
    exit 1
fi

echo "Output file found ($(wc -c < "$OUTPUT_FILE") bytes)"

# Extract log record bodies (messages) from the OTLP JSON output.
# The file exporter writes one JSON object per line (or one large JSON).
# Each line contains resourceLogs[].scopeLogs[].logRecords[].body.stringValue
ACTUAL_MESSAGES=$(jq -r '
  [.resourceLogs[]?.scopeLogs[]?.logRecords[]? |
    .body.stringValue // empty] | .[]
' "$OUTPUT_FILE" 2>/dev/null | sort)

EXPECTED_MESSAGES=$(jq -r '.[].message' "$EXPECTED_FILE" | sort)

if [ -z "$ACTUAL_MESSAGES" ]; then
    echo "FAIL: No log record messages found in output."
    echo "Output file content (first 2000 chars):"
    head -c 2000 "$OUTPUT_FILE"
    exit 1
fi

echo ""
echo "--- Expected messages ---"
echo "$EXPECTED_MESSAGES"
echo ""
echo "--- Actual messages ---"
echo "$ACTUAL_MESSAGES"
echo ""

# Compare messages
MISSING=0
while IFS= read -r msg; do
    if ! echo "$ACTUAL_MESSAGES" | grep -qF "$msg"; then
        echo "MISSING: $msg"
        MISSING=$((MISSING + 1))
    fi
done <<< "$EXPECTED_MESSAGES"

EXPECTED_COUNT=$(echo "$EXPECTED_MESSAGES" | wc -l)
ACTUAL_COUNT=$(echo "$ACTUAL_MESSAGES" | wc -l)

echo "Expected records: $EXPECTED_COUNT"
echo "Actual records:   $ACTUAL_COUNT"
echo "Missing records:  $MISSING"

if [ "$MISSING" -gt 0 ]; then
    echo ""
    echo "FAIL: $MISSING expected records not found in output."
    exit 1
fi

# Also validate that sources are present as attributes
ACTUAL_SOURCES=$(jq -r '
  [.resourceLogs[]?.scopeLogs[]?.logRecords[]? |
    (.attributes[]? | select(.key == "opcua.source") | .value.stringValue) // empty] | .[]
' "$OUTPUT_FILE" 2>/dev/null | sort -u)

EXPECTED_SOURCES=$(jq -r '.[].source' "$EXPECTED_FILE" | sort -u)

echo ""
echo "--- Expected sources ---"
echo "$EXPECTED_SOURCES"
echo "--- Actual sources ---"
echo "$ACTUAL_SOURCES"

SOURCE_MISSING=0
while IFS= read -r src; do
    if ! echo "$ACTUAL_SOURCES" | grep -qF "$src"; then
        echo "MISSING source: $src"
        SOURCE_MISSING=$((SOURCE_MISSING + 1))
    fi
done <<< "$EXPECTED_SOURCES"

if [ "$SOURCE_MISSING" -gt 0 ]; then
    echo ""
    echo "FAIL: $SOURCE_MISSING expected sources not found in output."
    exit 1
fi

echo ""
echo "PASS: All expected records and sources found in collector output."
exit 0

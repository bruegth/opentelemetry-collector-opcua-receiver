#!/bin/bash
# Validates the OTel collector file export against expected test records.
# Usage: ./validate.sh <output-file> <expected-file> [max-wait-seconds]
#
# The output file contains OTLP JSON log export (single JSON or NDJSON).
# The expected file contains an array of expected records with known fields.

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

# Parse all log records from the OTLP output.
# -s (slurp) handles both single-JSON and NDJSON (one JSON object per line) formats.
# Each record is normalized to a flat structure keyed by message body.
ACTUAL_RECORDS=$(jq -sc '
  [ .[].resourceLogs[]?.scopeLogs[]?.logRecords[]? | {
    message:          (.body.stringValue // ""),
    severity_number:  (.severityNumber // 0),
    severity_text:    (.severityText // ""),
    trace_id:         (.traceId // ""),
    span_id:          (.spanId // ""),
    source_name:      ((.attributes // []) | map(select(.key == "opcua.source.name"))     | first | .value.stringValue // ""),
    source_namespace: ((.attributes // []) | map(select(.key == "opcua.source.namespace"))| first | .value.intValue    // ""),
    source_id_type:   ((.attributes // []) | map(select(.key == "opcua.source.id_type"))  | first | .value.stringValue // ""),
    source_id:        ((.attributes // []) | map(select(.key == "opcua.source.id"))       | first | .value.stringValue // ""),
    attrs:            ((.attributes // []) | map({ (.key): (.value.stringValue // .value.intValue // "") }) | add // {})
  }]
' "$OUTPUT_FILE")

EXPECTED_COUNT=$(jq 'length' "$EXPECTED_FILE")
ACTUAL_COUNT=$(echo "$ACTUAL_RECORDS" | jq 'length')

echo ""
echo "Expected record count: $EXPECTED_COUNT"
echo "Actual record count:   $ACTUAL_COUNT"

# Run per-record field validation using jq.
# For each expected record find the matching actual record by message body and
# compare: severity_text, source_name, source_namespace, source_id_type,
# source_id, trace_id, span_id, and every custom attribute in .attributes.
#
# Note: source_namespace is stored as an OTLP intValue (JSON string "1") in the
# actual output, but as a JSON number in the expected file — compare via tostring.
FAILURES=$(jq -r --argjson actual "$ACTUAL_RECORDS" '
  ($actual | map({(.message): .}) | add) as $by_msg |
  .[] |
  . as $exp |
  ($by_msg[$exp.message] // null) as $act |
  if $act == null then
    "MISSING: \($exp.message)"
  else
    # severity_number — OTel SeverityNumber (1–24), NOT the raw OPC UA severity value
    (if $act.severity_number != $exp.severity_number then
      "FAIL [\($exp.message)]: severity_number: expected \($exp.severity_number) (OTel), got \($act.severity_number)"
    else empty end),
    # severity_text
    (if $act.severity_text != $exp.severity_text then
      "FAIL [\($exp.message)]: severity_text: expected \"\($exp.severity_text)\", got \"\($act.severity_text)\""
    else empty end),
    # source_name
    (if $act.source_name != $exp.source_name then
      "FAIL [\($exp.message)]: source_name: expected \"\($exp.source_name)\", got \"\($act.source_name)\""
    else empty end),
    # source_namespace: expected is JSON number, actual is intValue string — compare via tostring
    (if $act.source_namespace != ($exp.source_namespace | tostring) then
      "FAIL [\($exp.message)]: source_namespace: expected \($exp.source_namespace), got \"\($act.source_namespace)\""
    else empty end),
    # source_id_type
    (if $act.source_id_type != $exp.source_id_type then
      "FAIL [\($exp.message)]: source_id_type: expected \"\($exp.source_id_type)\", got \"\($act.source_id_type)\""
    else empty end),
    # source_id
    (if $act.source_id != $exp.source_id then
      "FAIL [\($exp.message)]: source_id: expected \"\($exp.source_id)\", got \"\($act.source_id)\""
    else empty end),
    # trace_id — only validate when the expected record carries one
    (if ($exp.trace_id // "") != "" then
      if $act.trace_id != $exp.trace_id then
        "FAIL [\($exp.message)]: trace_id: expected \"\($exp.trace_id)\", got \"\($act.trace_id)\""
      else empty end
    else empty end),
    # span_id — only validate when the expected record carries one
    (if ($exp.span_id // "") != "" then
      if $act.span_id != $exp.span_id then
        "FAIL [\($exp.message)]: span_id: expected \"\($exp.span_id)\", got \"\($act.span_id)\""
      else empty end
    else empty end),
    # custom attributes (AdditionalData) — iterate over every key in expected .attributes
    ($exp.attributes | to_entries[] |
      . as $entry |
      if $act.attrs[$entry.key] == null then
        "FAIL [\($exp.message)]: attribute \"\($entry.key)\" missing in output"
      elif $act.attrs[$entry.key] != $entry.value then
        "FAIL [\($exp.message)]: attribute \"\($entry.key)\": expected \"\($entry.value)\", got \"\($act.attrs[$entry.key])\""
      else empty end
    )
  end
' "$EXPECTED_FILE")

if [ -n "$FAILURES" ]; then
    echo ""
    echo "$FAILURES"
    FAILURE_COUNT=$(echo "$FAILURES" | wc -l)
    echo ""
    echo "FAIL: $FAILURE_COUNT validation error(s) found."
    exit 1
fi

echo ""
echo "PASS: All $EXPECTED_COUNT expected records validated successfully."
echo "      Validated: message, severity_number (OTel 1-24), severity_text,"
echo "                 source_name, source_namespace, source_id_type, source_id,"
echo "                 trace_id, span_id, attributes"
exit 0

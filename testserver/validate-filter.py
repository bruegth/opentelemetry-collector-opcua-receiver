#!/usr/bin/env python3
"""Validate severity filtering and the OTel processor against C# server fixtures."""

import json
import sys
from collections import Counter
from pathlib import Path


def records(path):
    with path.open(encoding="utf-8") as stream:
        return [
            record
            for line in stream
            for resource in json.loads(line).get("resourceLogs", [])
            for scope in resource.get("scopeLogs", [])
            for record in scope.get("logRecords", [])
        ]


def verify(actual, expected, *, allow_repeated=False):
    messages = [record["body"]["stringValue"] for record in actual]
    expected_by_message = {record["message"]: record for record in expected}
    counts = Counter(messages)
    if set(counts) != set(expected_by_message):
        raise AssertionError(f"unexpected messages: got {counts}, expected {list(expected_by_message)}")
    if not allow_repeated and any(count != 1 for count in counts.values()):
        raise AssertionError(f"unexpected repeated messages: {counts}")
    for record in actual:
        expected_record = expected_by_message[record["body"]["stringValue"]]
        if record.get("severityNumber") != expected_record["severity_number"]:
            raise AssertionError(f"unexpected severity: {record}")


def main(output_dir, expected_path, server_log):
    expected = json.loads(expected_path.read_text(encoding="utf-8"))
    accepted = [record for record in expected if record["severity"] >= 151]
    rejected = [record for record in expected if record["severity"] < 151]
    events = server_log.read_text(encoding="utf-8")
    for record in rejected:
        event = f'severity={record["severity"]} msg={record["message"]}'
        if event not in events:
            raise AssertionError(f"server did not emit rejected event during the test: {event}")

    verify(records(output_dir / "filter-poll.json"), accepted)
    verify(records(output_dir / "filter-subscription.json"), accepted, allow_repeated=True)
    verify(records(output_dir / "filter-processor.json"),
           [record for record in accepted if record["source_name"] == "NetworkModule"])
    print("PASS: polling and subscription severity filtering and OTel filtering")


if __name__ == "__main__":
    if len(sys.argv) != 4:
        raise SystemExit("usage: validate-filter.py OUTPUT_DIR EXPECTED_JSON SERVER_EVENT_LOG")
    main(*(Path(arg) for arg in sys.argv[1:]))

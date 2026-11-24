# OPC UA Part 26 review

Reviewed against OPC Foundation OPC-10000-26 **1.05.07** (15 April 2026), its
published NodeIds and the OpenTelemetry Collector stability guidance. This is
an implementation review, not an OPC UA conformance certification. Beta is the
requested project status; production exposure and the Collector's beta graduation
criteria have not been independently established.

## Confirmed and corrected

- Polling requests the optional fields, decodes the standard `LogRecordsDataType`
  wrapper and keeps the request parameters unchanged across continuation calls.
  SDK 2.0.2 binary fixtures and the included test server exercise this path.
- The standard ServerLog is `ns=0;i=19372`; the former default `i=2042` is
  `BaseEventType_EventId`, not a LogObject. A GetRecords method must be found
  on the selected object; the former fallback `i=11550` is not that method.
- Severity thresholds now include the requested Part 26 severity range, and
  Part 26 trace contexts set the random-ID flag without inventing a sampled flag.
- Endpoint selection now requires the configured policy and mode to match.

## Open issues

1. **Polling can lose or repeat records.**
   [client.go](receiver/opcua/client.go) stops at the per-node cap even when a
   continuation point remains, while [scraper.go](receiver/opcua/scraper.go)
   advances to the end of the entire time window. Unread records can therefore
   be skipped. The inclusive start boundary can also repeat records, and the
   cursor is not persisted across restarts. [receiver.go](receiver/opcua/receiver.go)
   advances the cursor before the downstream consumer accepts the logs.
   Design per-node paging/checkpointing, same-timestamp handling, downstream
   acknowledgement and optional continuation-point release together.
2. **Subscription event selection is not generally Part 26 compliant.**
   [subscription_receiver.go](receiver/opcua/subscription_receiver.go) checks
   `ConditionClassId` on the notifier rather than filtering each event by its
   class/subclass; it also monitors the configured LogObject as an event notifier.
   Those properties and notifier capabilities are not mandatory LogObject
   properties. The included [test server](testserver/TestNodeManager.cs) adds
   compatibility properties, so its successful event test does not prove
   interoperability. Redesign discovery, event filtering and notifier selection;
   use concrete event types with the mandatory event fields in the test server.
3. **Subscription payload and recovery need further work.**
   The subscription decoder expects a legacy LogRecord object for TraceContext
   instead of the standard TraceContextDataType; additional event fields and
   severity filtering are incomplete. Failed monitored items and downstream
   delivery/reconnection need explicit failure and replay semantics. Test
   independently against conforming servers, not just the included server.
4. **Configuration/security support is incomplete.**
   `filter.max_log_records` is validated but not enforced. `tls.ca_file` and
   `tls.insecure_skip_verify` are not passed to the OPC UA client. Endpoint token
   selection currently requests an anonymous token regardless of configured user
   authentication, and client certificates are not explicitly configured as a
   user identity. Validate secure-channel trust and all authentication modes
   against independently configured servers before relying on them.
5. **Discovery and mapping are limited.**
   Arbitrary browse paths and the Part 26 Logs folder are not traversed; only
   NodeIDs and known ServerLog aliases are resolved. Discovery may fall back to
   ServerLog or skip unresolved configured nodes. Polling does not export
   EventType or parent trace context, and some AdditionalData values are
   stringified. Zero-span trace contexts are dropped; assess Annex C.2's
   grouping guidance and validate GUID/trace-ID byte ordering across independent SDKs.

## References

- [Part 26, LogObject and GetRecords (§5)](https://reference.opcfoundation.org/specs/OPC-10000-26/5)
- [Part 26, LogObject and Events (§6)](https://reference.opcfoundation.org/specs/OPC-10000-26/6)
- [Part 26, standard instances (§7)](https://reference.opcfoundation.org/specs/OPC-10000-26/7)
- [Part 26, OpenTelemetry import (Annex C.2)](https://reference.opcfoundation.org/specs/OPC-10000-26/c-2)
- [OPC Foundation NodeIds, 1.05.07](https://github.com/OPCFoundation/UA-Nodeset/blob/UA-1.05.07-2026-04-15/Schema/NodeIds.csv)
- [W3C Trace Context Level 2, trace flags](https://www.w3.org/TR/trace-context-2/#trace-flags)
- [OpenTelemetry Collector component stability](https://github.com/open-telemetry/opentelemetry-collector/blob/main/docs/component-stability.md#beta)

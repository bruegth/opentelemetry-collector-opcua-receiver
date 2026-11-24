// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"fmt"

	"github.com/gopcua/opcua/ua"
)

const (
	logRecordEventTypeMask = 1 << iota
	logRecordSourceNodeMask
	logRecordSourceNameMask
	logRecordTraceContextMask
	logRecordAdditionalDataMask
	logRecordAllFieldsMask = (1 << iota) - 1
)

var (
	logRecordBinaryTypeID  = ua.NewNumericNodeID(0, 19379)
	logRecordsBinaryTypeID = ua.NewNumericNodeID(0, 19753)
)

// standardLogRecord decodes a Part 26 LogRecord, including its optional-field mask.
type standardLogRecord LogRecordExtObj

// logRecordsDataType contains inline LogRecords, not ExtensionObjects.
type logRecordsDataType struct {
	Records []standardLogRecord
}

func init() {
	ua.RegisterExtensionObject(logRecordBinaryTypeID, new(standardLogRecord))
	ua.RegisterExtensionObject(logRecordsBinaryTypeID, new(logRecordsDataType))
}

func (r *logRecordsDataType) Decode(data []byte) (int, error) {
	buf := ua.NewBuffer(data)
	count := buf.ReadInt32()
	if err := buf.Error(); err != nil {
		return buf.Pos(), err
	}
	if count < -1 || int64(count) > int64(ua.MaxVariantArrayLength) || int64(count) > int64(buf.Len()/15) {
		return buf.Pos(), fmt.Errorf("invalid LogRecord array length %d", count)
	}
	r.Records = nil
	if count > 0 {
		r.Records = make([]standardLogRecord, count)
		for i := range r.Records {
			buf.ReadStruct(&r.Records[i])
			if err := buf.Error(); err != nil {
				return buf.Pos(), fmt.Errorf("decode LogRecord %d: %w", i, err)
			}
		}
	}
	if buf.Len() != 0 {
		return buf.Pos(), fmt.Errorf("unexpected %d trailing bytes in LogRecordsDataType", buf.Len())
	}
	return buf.Pos(), nil
}

func (r *standardLogRecord) Decode(data []byte) (int, error) {
	*r = standardLogRecord{}
	buf := ua.NewBuffer(data)
	mask := buf.ReadUint32()
	if err := buf.Error(); err != nil {
		return buf.Pos(), err
	}
	if mask & ^uint32(logRecordAllFieldsMask) != 0 {
		return buf.Pos(), fmt.Errorf("unsupported LogRecord encoding mask %#x", mask)
	}

	r.Time = buf.ReadTime()
	r.Severity = buf.ReadUint16()
	if mask&logRecordEventTypeMask != 0 {
		r.EventTypeNode = new(ua.NodeID)
		buf.ReadStruct(r.EventTypeNode)
	}
	if mask&logRecordSourceNodeMask != 0 {
		r.SourceNode = new(ua.NodeID)
		buf.ReadStruct(r.SourceNode)
	}
	if mask&logRecordSourceNameMask != 0 {
		r.SourceName = buf.ReadString()
	}
	message := new(ua.LocalizedText)
	buf.ReadStruct(message)
	r.Message = message.Text
	if mask&logRecordTraceContextMask != 0 {
		copy(r.TraceIDBytes[:], buf.ReadN(16))
		r.SpanID = buf.ReadUint64()
		r.ParentSpanID = buf.ReadUint64()
		r.ParentIdentifier = buf.ReadString()
	}
	if mask&logRecordAdditionalDataMask != 0 {
		count := buf.ReadInt32()
		if err := buf.Error(); err != nil {
			return buf.Pos(), err
		}
		if count < -1 || int64(count) > int64(ua.MaxVariantArrayLength) || int64(count) > int64(buf.Len()/5) {
			return buf.Pos(), fmt.Errorf("invalid AdditionalData array length %d", count)
		}
		if count > 0 {
			r.AdditionalData = make(map[string]interface{}, count)
			for i := int32(0); i < count; i++ {
				name := buf.ReadString()
				value := new(ua.Variant)
				buf.ReadStruct(value)
				if err := buf.Error(); err != nil {
					return buf.Pos(), fmt.Errorf("decode AdditionalData %d: %w", i, err)
				}
				r.AdditionalData[name] = value.Value()
			}
		}
	}
	return buf.Pos(), buf.Error()
}

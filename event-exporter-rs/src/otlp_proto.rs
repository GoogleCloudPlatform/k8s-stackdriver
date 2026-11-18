//! Pure-Rust `prost` message definitions for OpenTelemetry OTLP Logs v1
//! (`opentelemetry.proto.{common,resource,logs,collector.logs}.v1`), matching upstream `.proto`
//! wire tags 1:1 without requiring an external `protoc` compiler at build time.

/// `opentelemetry.proto.common.v1.AnyValue`
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AnyValue {
    #[prost(oneof = "any_value::Value", tags = "1, 2, 3, 4, 5, 6, 7")]
    pub value: Option<any_value::Value>,
}

pub mod any_value {
    #[derive(Clone, PartialEq, ::prost::Oneof)]
    pub enum Value {
        #[prost(string, tag = "1")]
        StringValue(String),
        #[prost(bool, tag = "2")]
        BoolValue(bool),
        #[prost(int64, tag = "3")]
        IntValue(i64),
        #[prost(double, tag = "4")]
        DoubleValue(f64),
        #[prost(message, tag = "5")]
        ArrayValue(super::ArrayValue),
        #[prost(message, tag = "6")]
        KvlistValue(super::KeyValueList),
        #[prost(bytes, tag = "7")]
        BytesValue(Vec<u8>),
    }
}

/// `opentelemetry.proto.common.v1.ArrayValue`
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ArrayValue {
    #[prost(message, repeated, tag = "1")]
    pub values: Vec<AnyValue>,
}

/// `opentelemetry.proto.common.v1.KeyValueList`
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct KeyValueList {
    #[prost(message, repeated, tag = "1")]
    pub values: Vec<KeyValue>,
}

/// `opentelemetry.proto.common.v1.KeyValue`
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct KeyValue {
    #[prost(string, tag = "1")]
    pub key: String,
    #[prost(message, optional, tag = "2")]
    pub value: Option<AnyValue>,
}

impl KeyValue {
    pub fn string(key: impl Into<String>, val: impl Into<String>) -> Self {
        Self {
            key: key.into(),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue(val.into())),
            }),
        }
    }
}

/// `opentelemetry.proto.common.v1.InstrumentationScope`
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct InstrumentationScope {
    #[prost(string, tag = "1")]
    pub name: String,
    #[prost(string, tag = "2")]
    pub version: String,
    #[prost(message, repeated, tag = "3")]
    pub attributes: Vec<KeyValue>,
    #[prost(uint32, tag = "4")]
    pub dropped_attributes_count: u32,
}

/// `opentelemetry.proto.resource.v1.Resource`
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct Resource {
    #[prost(message, repeated, tag = "1")]
    pub attributes: Vec<KeyValue>,
    #[prost(uint32, tag = "2")]
    pub dropped_attributes_count: u32,
}

/// `opentelemetry.proto.logs.v1.SeverityNumber`
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum SeverityNumber {
    Unspecified = 0,
    Trace = 1,
    Debug = 5,
    Info = 9,
    Warn = 13,
    Error = 17,
    Fatal = 21,
}

/// `opentelemetry.proto.logs.v1.LogRecord`
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct LogRecord {
    #[prost(fixed64, tag = "1")]
    pub time_unix_nano: u64,
    #[prost(enumeration = "SeverityNumber", tag = "2")]
    pub severity_number: i32,
    #[prost(string, tag = "3")]
    pub severity_text: String,
    #[prost(message, optional, tag = "5")]
    pub body: Option<AnyValue>,
    #[prost(message, repeated, tag = "6")]
    pub attributes: Vec<KeyValue>,
    #[prost(uint32, tag = "7")]
    pub dropped_attributes_count: u32,
    #[prost(fixed32, tag = "8")]
    pub flags: u32,
    #[prost(bytes = "vec", tag = "9")]
    pub trace_id: Vec<u8>,
    #[prost(bytes = "vec", tag = "10")]
    pub span_id: Vec<u8>,
    #[prost(fixed64, tag = "11")]
    pub observed_time_unix_nano: u64,
    #[prost(string, tag = "12")]
    pub event_name: String,
}

/// `opentelemetry.proto.logs.v1.ScopeLogs`
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ScopeLogs {
    #[prost(message, optional, tag = "1")]
    pub scope: Option<InstrumentationScope>,
    #[prost(message, repeated, tag = "2")]
    pub log_records: Vec<LogRecord>,
    #[prost(string, tag = "3")]
    pub schema_url: String,
}

/// `opentelemetry.proto.logs.v1.ResourceLogs`
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ResourceLogs {
    #[prost(message, optional, tag = "1")]
    pub resource: Option<Resource>,
    #[prost(message, repeated, tag = "2")]
    pub scope_logs: Vec<ScopeLogs>,
    #[prost(string, tag = "3")]
    pub schema_url: String,
}

/// `opentelemetry.proto.collector.logs.v1.ExportLogsServiceRequest`
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ExportLogsServiceRequest {
    #[prost(message, repeated, tag = "1")]
    pub resource_logs: Vec<ResourceLogs>,
}

/// `opentelemetry.proto.collector.logs.v1.ExportLogsPartialSuccess`
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ExportLogsPartialSuccess {
    #[prost(int64, tag = "1")]
    pub rejected_log_records: i64,
    #[prost(string, tag = "2")]
    pub error_message: String,
}

/// `opentelemetry.proto.collector.logs.v1.ExportLogsServiceResponse`
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ExportLogsServiceResponse {
    #[prost(message, optional, tag = "1")]
    pub partial_success: Option<ExportLogsPartialSuccess>,
}

// SPDX-License-Identifier: MIT
// Independently reusable HTTP message definitions; see PROTOCOL-LICENSE.
//! Public astronomy-only protocol. No asset registry or financial semantics.
use serde::{Deserialize, Serialize};
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Request {
    pub timestamp_ms: i64,
    pub latitude: Option<f64>,
    pub longitude: Option<f64>,
}
#[derive(Clone, Copy, Debug, Serialize, Deserialize)]
pub struct Position {
    pub longitude: f64,
    pub latitude: f64,
    pub speed_longitude: f64,
    pub speed_latitude: f64,
    pub declination: f64,
    pub declination_speed: f64,
}
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Response {
    pub planets: [Position; 10],
    pub true_node: f64,
    pub house_cusps: Option<Vec<f64>>,
}

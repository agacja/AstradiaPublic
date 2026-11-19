//! Outbound HTTP helpers: a client that never follows redirects, and a bounded
//! response reader so a remote service cannot make this process buffer an
//! unbounded body. Error values contain no upstream content.
use serde_json::Value;
use std::time::Duration;

#[derive(Debug, thiserror::Error)]
pub enum HttpError {
    #[error("Upstream unavailable")]
    Unavailable,
    #[error("Upstream response invalid")]
    Data,
}
pub type Result<T> = std::result::Result<T, HttpError>;

pub fn http_client() -> Result<reqwest::Client> {
    reqwest::Client::builder()
        .timeout(Duration::from_secs(30))
        .connect_timeout(Duration::from_secs(10))
        .redirect(reqwest::redirect::Policy::none())
        .build()
        .map_err(|_| HttpError::Unavailable)
}

pub async fn bounded_json(mut response: reqwest::Response, max: usize) -> Result<Value> {
    let mut bytes = Vec::new();
    while let Some(chunk) = response.chunk().await.map_err(|_| HttpError::Unavailable)? {
        if bytes.len() + chunk.len() > max {
            return Err(HttpError::Data);
        }
        bytes.extend_from_slice(&chunk);
    }
    serde_json::from_slice(&bytes).map_err(|_| HttpError::Data)
}

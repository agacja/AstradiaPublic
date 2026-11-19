//! Payment-provider notifications are verified before any database write.
//! Both checks fail closed on a missing secret and compare in constant time.
use hmac::{Hmac, Mac};
use serde_json::Value;
use sha2::{Sha256, Sha512};

pub fn verify_stripe(body: &[u8], header: &str, secret: &[u8], now: i64) -> bool {
    let fields: Vec<_> = header
        .split(',')
        .filter_map(|v| v.split_once('='))
        .collect();
    let ts: Vec<_> = fields.iter().filter(|(k, _)| *k == "t").collect();
    if ts.len() != 1 || secret.is_empty() {
        return false;
    }
    let Ok(time) = ts[0].1.parse::<i64>() else {
        return false;
    };
    if now.abs_diff(time) > 300 {
        return false;
    }
    let mut mac = Hmac::<Sha256>::new_from_slice(secret).expect("HMAC accepts arbitrary length");
    mac.update(ts[0].1.as_bytes());
    mac.update(b".");
    mac.update(body);
    fields
        .iter()
        .filter(|(k, _)| *k == "v1")
        .any(|(_, sig)| hex::decode(sig).is_ok_and(|sig| mac.clone().verify_slice(&sig).is_ok()))
}
pub fn verify_now(body: &Value, signature: &str, secret: &[u8]) -> bool {
    if secret.is_empty() {
        return false;
    }
    let Ok(canonical) = serde_json_canonicalizer::to_vec(body) else {
        return false;
    };
    let Ok(signature) = hex::decode(signature) else {
        return false;
    };
    let mut mac = Hmac::<Sha512>::new_from_slice(secret).expect("HMAC accepts arbitrary length");
    mac.update(&canonical);
    mac.verify_slice(&signature).is_ok()
}
#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;
    #[test]
    fn stripe_requires_exact_bytes_and_fresh_timestamp() {
        let body = br#"{"id":"evt_test"}"#;
        let mut mac = Hmac::<Sha256>::new_from_slice(b"test-secret").unwrap();
        mac.update(b"1000.");
        mac.update(body);
        let header = format!("t=1000,v1={}", hex::encode(mac.finalize().into_bytes()));
        assert!(verify_stripe(body, &header, b"test-secret", 1001));
        assert!(!verify_stripe(body, &header, b"test-secret", 1301));
        assert!(!verify_stripe(b"different", &header, b"test-secret", 1001));
        assert!(!verify_stripe(body, &header, b"", 1001));
    }
    #[test]
    fn now_signature_is_order_independent_and_fails_closed() {
        let a = json!({"z":1,"a":{"d":2,"b":3}});
        let mut mac = Hmac::<Sha512>::new_from_slice(b"test-secret").unwrap();
        mac.update(br#"{"a":{"b":3,"d":2},"z":1}"#);
        let sig = hex::encode(mac.finalize().into_bytes());
        assert!(verify_now(&a, &sig, b"test-secret"));
        assert!(!verify_now(&json!({"z":2}), &sig, b"test-secret"));
        assert!(!verify_now(&a, &sig, b""));
    }
}

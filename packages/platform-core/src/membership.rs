//! Membership checks are separate from authentication. Social consent remains
//! owned by the social service; a subscription never substitutes for consent.
use chrono::{DateTime, Utc};
use serde_json::Value;

/// Sensitive columns never enter this object. Public responses use explicit DTOs.
#[derive(Clone)]
pub struct User {
    pub data: Value,
}
impl User {
    pub fn text(&self, k: &str) -> &str {
        self.data[k].as_str().unwrap_or("")
    }
    pub fn has_subscription_at(&self, now: DateTime<Utc>) -> bool {
        if self.text("subscription_status") != "active" {
            return false;
        }
        match self.data.get("subscription_expires_at") {
            None | Some(Value::Null) => true,
            Some(value) => value
                .as_str()
                .and_then(timestamp)
                .is_some_and(|expires| expires > now),
        }
    }
}
pub fn timestamp(raw: &str) -> Option<DateTime<Utc>> {
    DateTime::parse_from_rfc3339(raw)
        .ok()
        .map(|d| d.with_timezone(&Utc))
        .or_else(|| {
            chrono::NaiveDateTime::parse_from_str(raw, "%Y-%m-%dT%H:%M:%S%.f")
                .ok()
                .map(|d| d.and_utc())
        })
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn subscription_requires_active_status_and_unexpired_valid_date() {
        let now = "2026-11-09T12:00:00Z".parse().unwrap();
        for (status, expires, allowed) in [
            ("none", None, false),
            ("expired", None, false),
            ("cancelled", Some("2026-12-01T00:00:00Z"), false),
            ("active", None, true),
            ("active", Some("2026-12-01T00:00:00Z"), true),
            ("active", Some("2026-11-09T12:00:00Z"), false),
            ("active", Some("2026-11-08T12:00:00Z"), false),
            ("active", Some("invalid"), false),
        ] {
            let u = User {
                data: json!({"subscription_status":status,"subscription_expires_at":expires}),
            };
            assert_eq!(u.has_subscription_at(now), allowed, "{status} {expires:?}");
        }
    }
}

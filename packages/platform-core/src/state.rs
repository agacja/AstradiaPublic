//! Durable state, compatible with Go/TS bot:kv keys. No live in-memory fallback.
use async_trait::async_trait;
use secrecy::{ExposeSecret, SecretString};
use serde::{Serialize, de::DeserializeOwned};
use serde_json::Value;
use std::{collections::HashMap, sync::Arc};
use tokio::sync::Mutex;
#[derive(Debug, thiserror::Error)]
#[error("Persistent state unavailable")]
pub struct StateError;
pub type Result<T> = std::result::Result<T, StateError>;
#[async_trait]
pub trait Store: Send + Sync {
    async fn get(&self, key: &str) -> Result<Option<Value>>;
    async fn put(&self, key: &str, value: Value, ttl: u64) -> Result<()>;
    async fn delete(&self, key: &str) -> Result<()>;
    async fn lease(&self, key: &str, owner: &str, ttl_ms: u64) -> Result<bool>;
    async fn release(&self, key: &str, owner: &str) -> Result<()>;
    async fn nonce(&self, signer: &str, now: u64) -> Result<u64>;
}
pub async fn read<T: DeserializeOwned>(s: &dyn Store, key: &str) -> Result<Option<T>> {
    s.get(key)
        .await?
        .map(serde_json::from_value)
        .transpose()
        .map_err(|_| StateError)
}
pub async fn write(s: &dyn Store, key: &str, value: &impl Serialize, ttl: u64) -> Result<()> {
    s.put(
        key,
        serde_json::to_value(value).map_err(|_| StateError)?,
        ttl,
    )
    .await
}
#[derive(Clone)]
pub struct RedisStore {
    connection: redis::aio::ConnectionManager,
}
impl RedisStore {
    pub async fn connect(url: &SecretString) -> Result<Self> {
        let client = redis::Client::open(url.expose_secret()).map_err(|_| StateError)?;
        let connection = client
            .get_connection_manager()
            .await
            .map_err(|_| StateError)?;
        Ok(Self { connection })
    }
}
#[async_trait]
impl Store for RedisStore {
    async fn get(&self, key: &str) -> Result<Option<Value>> {
        let text: Option<String> = redis::cmd("GET")
            .arg(format!("bot:kv:{key}"))
            .query_async(&mut self.connection.clone())
            .await
            .map_err(|_| StateError)?;
        text.map(|t| serde_json::from_str(&t))
            .transpose()
            .map_err(|_| StateError)
    }
    async fn put(&self, key: &str, value: Value, ttl: u64) -> Result<()> {
        let mut command = redis::cmd("SET");
        command
            .arg(format!("bot:kv:{key}"))
            .arg(serde_json::to_string(&value).map_err(|_| StateError)?);
        if ttl > 0 {
            command.arg("EX").arg(ttl);
        }
        command
            .query_async::<()>(&mut self.connection.clone())
            .await
            .map_err(|_| StateError)
    }
    async fn delete(&self, key: &str) -> Result<()> {
        redis::cmd("DEL")
            .arg(format!("bot:kv:{key}"))
            .query_async::<i64>(&mut self.connection.clone())
            .await
            .map(|_| ())
            .map_err(|_| StateError)
    }
    async fn lease(&self, key: &str, owner: &str, ttl_ms: u64) -> Result<bool> {
        let answer:i64=redis::Script::new("if redis.call('GET',KEYS[1]) == ARGV[1] then redis.call('PEXPIRE',KEYS[1],ARGV[2]); return 1 elseif redis.call('SET',KEYS[1],ARGV[1],'NX','PX',ARGV[2]) then return 1 else return 0 end").key(format!("bot:rust:lease:{key}")).arg(owner).arg(ttl_ms).invoke_async(&mut self.connection.clone()).await.map_err(|_|StateError)?;
        Ok(answer == 1)
    }
    async fn release(&self, key: &str, owner: &str) -> Result<()> {
        redis::Script::new("if redis.call('GET',KEYS[1]) == ARGV[1] then return redis.call('DEL',KEYS[1]) else return 0 end").key(format!("bot:rust:lease:{key}")).arg(owner).invoke_async::<i64>(&mut self.connection.clone()).await.map(|_|()).map_err(|_|StateError)
    }
    async fn nonce(&self, signer: &str, now: u64) -> Result<u64> {
        redis::Script::new("local n=math.max(tonumber(ARGV[1]),tonumber(redis.call('GET',KEYS[1]) or 0)+1);redis.call('SET',KEYS[1],n,'EX',172800);return n").key(format!("bot:rust:nonce:{signer}")).arg(now).invoke_async(&mut self.connection.clone()).await.map_err(|_|StateError)
    }
}
/// Offline replay and test store only. Production binaries require Redis.
#[derive(Default, Clone)]
pub struct MemoryStore {
    values: Arc<Mutex<HashMap<String, Value>>>,
    leases: Arc<Mutex<HashMap<String, (String, std::time::Instant)>>>,
    nonces: Arc<Mutex<HashMap<String, u64>>>,
}
#[async_trait]
impl Store for MemoryStore {
    async fn get(&self, k: &str) -> Result<Option<Value>> {
        Ok(self.values.lock().await.get(k).cloned())
    }
    async fn put(&self, k: &str, v: Value, _ttl: u64) -> Result<()> {
        self.values.lock().await.insert(k.into(), v);
        Ok(())
    }
    async fn delete(&self, k: &str) -> Result<()> {
        self.values.lock().await.remove(k);
        Ok(())
    }
    async fn lease(&self, k: &str, owner: &str, ttl: u64) -> Result<bool> {
        let mut l = self.leases.lock().await;
        let now = std::time::Instant::now();
        if l.get(k).is_some_and(|(o, t)| o != owner && *t > now) {
            return Ok(false);
        }
        l.insert(
            k.into(),
            (owner.into(), now + std::time::Duration::from_millis(ttl)),
        );
        Ok(true)
    }
    async fn release(&self, k: &str, owner: &str) -> Result<()> {
        let mut l = self.leases.lock().await;
        if l.get(k).is_some_and(|(o, _)| o == owner) {
            l.remove(k);
        }
        Ok(())
    }
    async fn nonce(&self, k: &str, now: u64) -> Result<u64> {
        let mut n = self.nonces.lock().await;
        let v = n.entry(k.into()).or_default();
        *v = now.max(*v + 1);
        Ok(*v)
    }
}

/// Shared account/configuration lease. Cancellation releases it; Redis TTL is
/// the fallback if the process dies. Worker leases are renewed separately.
pub struct Lease {
    store: Arc<dyn Store>,
    key: String,
    owner: String,
    active: bool,
}
impl Lease {
    pub async fn release(mut self) -> Result<()> {
        let result = self.store.release(&self.key, &self.owner).await;
        self.active = false;
        result
    }
    pub async fn acquire(store: Arc<dyn Store>, key: String, ttl_ms: u64) -> Result<Option<Self>> {
        let owner = uuid::Uuid::new_v4().to_string();
        if !store.lease(&key, &owner, ttl_ms).await? {
            return Ok(None);
        }
        Ok(Some(Self {
            store,
            key,
            owner,
            active: true,
        }))
    }
}
impl Drop for Lease {
    fn drop(&mut self) {
        if !self.active {
            return;
        }
        let store = self.store.clone();
        let key = self.key.clone();
        let owner = self.owner.clone();
        if let Ok(runtime) = tokio::runtime::Handle::try_current() {
            runtime.spawn(async move {
                let _ = store.release(&key, &owner).await;
            });
        }
    }
}

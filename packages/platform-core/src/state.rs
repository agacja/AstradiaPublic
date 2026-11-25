//! Durable state, compatible with Go/TS bot:kv keys. No live in-memory fallback.
use async_trait::async_trait;
use redis::aio::ConnectionManagerConfig;
use secrecy::{ExposeSecret, SecretString};
use serde::{Deserialize, Serialize, de::DeserializeOwned};
use serde_json::Value;
use std::{
    collections::{HashMap, HashSet},
    sync::Arc,
    time::Duration,
};
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
    /// The values of `keys`, in their order, each read exactly as `get` reads
    /// it: a value that cannot be read is an error for its own key only. Redis
    /// reads them all in one round trip; the default reads them one by one.
    async fn get_many(&self, keys: &[String]) -> Vec<Result<Option<Value>>> {
        let mut values = Vec::with_capacity(keys.len());
        for key in keys {
            values.push(self.get(key).await);
        }
        values
    }
    /// Adds `by` to the whole number kept at `key`, a missing key counting as
    /// zero, and returns the sum, in one atomic step: callers that add at the
    /// same time each get a sum of their own. A key this creates expires
    /// after `ttl` seconds (never, for 0); a key that exists keeps its expiry.
    /// The default refuses, because a store that cannot add atomically must
    /// not count anything.
    async fn add(&self, key: &str, by: i64, ttl: u64) -> Result<i64> {
        let _ = (key, by, ttl);
        Err(StateError)
    }
    /// Appends `entry` to `queue` under `id`, unless an entry with that id
    /// already waits there, and says whether it was added. An entry is added
    /// whole or not at all, whatever stops the call part-way. Entries are kept
    /// until they are dequeued.
    ///
    /// The default keeps the whole queue as one value under `queue`, rewritten
    /// on every change: fine for the in-memory store, where a queue has one
    /// writer at a time. Redis keeps every entry apart, so adding or removing
    /// one costs the same however many wait.
    async fn enqueue(&self, queue: &str, id: &str, entry: Value) -> Result<bool> {
        let mut waiting = waiting(self.get(queue).await?)?;
        if waiting.iter().any(|w| w.id == id) {
            return Ok(false);
        }
        waiting.push(Waiting {
            id: id.into(),
            entry,
        });
        let value = serde_json::to_value(&waiting).map_err(|_| StateError)?;
        self.put(queue, value, 0).await?;
        Ok(true)
    }
    /// How many entries wait in `queue`.
    async fn queue_len(&self, queue: &str) -> Result<u64> {
        Ok(waiting(self.get(queue).await?)?.len() as u64)
    }
    /// The oldest entries waiting in `queue`, oldest first and at most
    /// `limit` of them, each with its id. Changes nothing.
    async fn queue_front(&self, queue: &str, limit: usize) -> Result<Vec<(String, Value)>> {
        let waiting = waiting(self.get(queue).await?)?;
        Ok(waiting
            .into_iter()
            .take(limit)
            .map(|w| (w.id, w.entry))
            .collect())
    }
    /// Removes the entry `id` from `queue`, if it waits there.
    async fn dequeue(&self, queue: &str, id: &str) -> Result<()> {
        let mut waiting = waiting(self.get(queue).await?)?;
        let before = waiting.len();
        waiting.retain(|w| w.id != id);
        if waiting.len() == before {
            return Ok(());
        }
        if waiting.is_empty() {
            return self.delete(queue).await;
        }
        let value = serde_json::to_value(&waiting).map_err(|_| StateError)?;
        self.put(queue, value, 0).await
    }
    /// What `key` holds, together with the exact form it is kept in, for a
    /// later `delete_if`. The default reads it with `get`.
    async fn get_stamped(&self, key: &str) -> Result<Option<Stamped>> {
        Ok(self.get(key).await?.map(Stamped::of))
    }
    /// Removes `key` only if it still holds exactly what `read` was, in one
    /// atomic step, and says whether it did. A key that changed since is left
    /// as it is, for the caller to read again. The default compares and then
    /// removes in two steps: fine for a store with one writer at a time.
    async fn delete_if(&self, key: &str, read: &Stamped) -> Result<bool> {
        let current = self.get(key).await?;
        if current.is_none_or(|value| stamp(&value) != read.stamp) {
            return Ok(false);
        }
        self.delete(key).await?;
        Ok(true)
    }
}
/// A value as it was read, with the exact form it was kept in.
#[derive(Clone, Debug)]
pub struct Stamped {
    pub value: Value,
    /// The stored text, which `delete_if` compares with what is there then.
    stamp: String,
}
impl Stamped {
    /// A value kept as it is, in a store that keeps values rather than text.
    fn of(value: Value) -> Self {
        let stamp = stamp(&value);
        Self { value, stamp }
    }
}
/// The stamp of a value kept as a value: its JSON text, which is the same
/// for equal values.
fn stamp(value: &Value) -> String {
    value.to_string()
}
/// Removes `KEYS[1]` only while it holds `ARGV[1]`, in one step. The same
/// script hands an account lease back.
const DELETE_IF_UNCHANGED: &str = "if redis.call('GET',KEYS[1]) == ARGV[1] then return redis.call('DEL',KEYS[1]) else return 0 end";
/// One queue entry, as the default queue keeps it.
#[derive(Serialize, Deserialize)]
struct Waiting {
    id: String,
    entry: Value,
}
fn waiting(value: Option<Value>) -> Result<Vec<Waiting>> {
    value
        .map(serde_json::from_value)
        .transpose()
        .map_err(|_| StateError)
        .map(Option::unwrap_or_default)
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
/// Redis must answer or fail: the worker's lease renewal and trading loop both
/// wait on this connection, and the library's default is to wait forever.
const RESPONSE_TIMEOUT: Duration = Duration::from_secs(5);
const CONNECTION_TIMEOUT: Duration = Duration::from_secs(5);
fn connection_config(response: Duration, connection: Duration) -> ConnectionManagerConfig {
    ConnectionManagerConfig::new()
        .set_response_timeout(response)
        .set_connection_timeout(connection)
}
impl RedisStore {
    pub async fn connect(url: &SecretString) -> Result<Self> {
        Self::connect_with(url, connection_config(RESPONSE_TIMEOUT, CONNECTION_TIMEOUT)).await
    }
    async fn connect_with(url: &SecretString, config: ConnectionManagerConfig) -> Result<Self> {
        let client = redis::Client::open(url.expose_secret()).map_err(|_| StateError)?;
        let connection = client
            .get_connection_manager_with_config(config)
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
    async fn get_many(&self, keys: &[String]) -> Vec<Result<Option<Value>>> {
        if keys.is_empty() {
            return Vec::new();
        }
        let mut command = redis::cmd("MGET");
        for key in keys {
            command.arg(format!("bot:kv:{key}"));
        }
        let texts = command
            .query_async::<Vec<Option<String>>>(&mut self.connection.clone())
            .await;
        match texts {
            Ok(texts) if texts.len() == keys.len() => texts
                .into_iter()
                .map(|text| {
                    text.map(|t| serde_json::from_str(&t))
                        .transpose()
                        .map_err(|_| StateError)
                })
                .collect(),
            _ => keys.iter().map(|_| Err(StateError)).collect(),
        }
    }
    async fn add(&self, key: &str, by: i64, ttl: u64) -> Result<i64> {
        let key = format!("bot:kv:{key}");
        let mut steps = redis::pipe();
        steps.atomic();
        // Creating the key with its expiry and adding to it are one step, so
        // no caller can see the sum of a key that never expires.
        if ttl > 0 {
            steps
                .cmd("SET")
                .arg(&key)
                .arg(0)
                .arg("EX")
                .arg(ttl)
                .arg("NX")
                .ignore();
        }
        steps.cmd("INCRBY").arg(&key).arg(by);
        let (sum,): (i64,) = steps
            .query_async(&mut self.connection.clone())
            .await
            .map_err(|_| StateError)?;
        Ok(sum)
    }
    async fn enqueue(&self, queue: &str, id: &str, entry: Value) -> Result<bool> {
        let (entries, order) = queue_keys(queue);
        let text = serde_json::to_string(&entry).map_err(|_| StateError)?;
        // The entry and its place in line are written together. An id that is
        // already waiting keeps its entry and gains a second place in line,
        // which the reads skip and its removal clears.
        let (added,): (i64,) = redis::pipe()
            .atomic()
            .cmd("HSETNX")
            .arg(&entries)
            .arg(id)
            .arg(text)
            .cmd("RPUSH")
            .arg(&order)
            .arg(id)
            .ignore()
            .query_async(&mut self.connection.clone())
            .await
            .map_err(|_| StateError)?;
        Ok(added == 1)
    }
    async fn queue_len(&self, queue: &str) -> Result<u64> {
        redis::cmd("HLEN")
            .arg(queue_keys(queue).0)
            .query_async(&mut self.connection.clone())
            .await
            .map_err(|_| StateError)
    }
    async fn queue_front(&self, queue: &str, limit: usize) -> Result<Vec<(String, Value)>> {
        let (entries, order) = queue_keys(queue);
        let mut found = Vec::new();
        let mut seen = HashSet::new();
        let mut start: usize = 0;
        if limit == 0 {
            return Ok(found);
        }
        loop {
            let ids: Vec<String> = redis::cmd("LRANGE")
                .arg(&order)
                .arg(start)
                .arg(start.saturating_add(limit - 1))
                .query_async(&mut self.connection.clone())
                .await
                .map_err(|_| StateError)?;
            if ids.is_empty() {
                break;
            }
            let texts: Vec<Option<String>> = redis::cmd("HMGET")
                .arg(&entries)
                .arg(&ids)
                .query_async(&mut self.connection.clone())
                .await
                .map_err(|_| StateError)?;
            if texts.len() != ids.len() {
                return Err(StateError);
            }
            for (id, text) in ids.iter().zip(texts) {
                // A place in line whose entry is gone, or a second place of
                // one already found, is passed over.
                let Some(text) = text else { continue };
                if !seen.insert(id.clone()) {
                    continue;
                }
                let entry = serde_json::from_str(&text).map_err(|_| StateError)?;
                found.push((id.clone(), entry));
                if found.len() == limit {
                    return Ok(found);
                }
            }
            if ids.len() < limit {
                break;
            }
            start = start.saturating_add(limit);
        }
        Ok(found)
    }
    async fn dequeue(&self, queue: &str, id: &str) -> Result<()> {
        let (entries, order) = queue_keys(queue);
        redis::pipe()
            .atomic()
            .cmd("HDEL")
            .arg(&entries)
            .arg(id)
            .ignore()
            .cmd("LREM")
            .arg(&order)
            .arg(0)
            .arg(id)
            .ignore()
            .query_async::<()>(&mut self.connection.clone())
            .await
            .map_err(|_| StateError)
    }
    /// The stamp is the text Redis holds, byte for byte.
    async fn get_stamped(&self, key: &str) -> Result<Option<Stamped>> {
        let text: Option<String> = redis::cmd("GET")
            .arg(format!("bot:kv:{key}"))
            .query_async(&mut self.connection.clone())
            .await
            .map_err(|_| StateError)?;
        text.map(|stamp| {
            let value = serde_json::from_str(&stamp).map_err(|_| StateError)?;
            Ok(Stamped { value, stamp })
        })
        .transpose()
    }
    async fn delete_if(&self, key: &str, read: &Stamped) -> Result<bool> {
        let removed: i64 = redis::cmd("EVAL")
            .arg(DELETE_IF_UNCHANGED)
            .arg(1)
            .arg(format!("bot:kv:{key}"))
            .arg(&read.stamp)
            .query_async(&mut self.connection.clone())
            .await
            .map_err(|_| StateError)?;
        Ok(removed == 1)
    }
}
/// Where Redis keeps a queue: its entries by id, and their ids in line.
fn queue_keys(queue: &str) -> (String, String) {
    (
        format!("bot:queue:{queue}:entries"),
        format!("bot:queue:{queue}:order"),
    )
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
    /// Atomic under the store's lock. Like `put`, it keeps no expiry.
    async fn add(&self, k: &str, by: i64, _ttl: u64) -> Result<i64> {
        let mut values = self.values.lock().await;
        let current = match values.get(k) {
            None => 0,
            Some(value) => value.as_i64().ok_or(StateError)?,
        };
        let sum = current.checked_add(by).ok_or(StateError)?;
        values.insert(k.into(), Value::from(sum));
        Ok(sum)
    }
    /// Atomic under the store's lock.
    async fn delete_if(&self, k: &str, read: &Stamped) -> Result<bool> {
        let mut values = self.values.lock().await;
        let unchanged = values
            .get(k)
            .is_some_and(|value| stamp(value) == read.stamp);
        if unchanged {
            values.remove(k);
        }
        Ok(unchanged)
    }
}

/// The reads of one account's cycle made ahead, several in one round trip.
/// Each value read ahead is handed to the first `get` of its key and then
/// forgotten; a later `get` reads the store again. Writing or removing a key
/// forgets what was read ahead for it, so a value this cycle changed is never
/// handed out. Everything else goes straight to the store.
///
/// So a value read ahead is the one a read at its usual place would find,
/// provided nothing outside the cycle changes the key in between. That is
/// what the caller guarantees by what it reads ahead: keys that only the
/// worker writes, or that every writer changes under the account lease the
/// cycle holds.
pub struct ReadAhead {
    store: Arc<dyn Store>,
    held: std::sync::Mutex<HashMap<String, Option<Value>>>,
}
impl ReadAhead {
    pub fn new(store: Arc<dyn Store>) -> Self {
        Self {
            store,
            held: std::sync::Mutex::default(),
        }
    }
    /// Reads `keys` in one round trip and keeps what could be read. A key
    /// that could not be read is read at its usual place instead.
    pub async fn fetch(&self, keys: &[String]) {
        if keys.is_empty() {
            return;
        }
        let values = self.store.get_many(keys).await;
        let mut held = self.lock();
        for (key, value) in keys.iter().zip(values) {
            if let Ok(value) = value {
                held.insert(key.clone(), value);
            }
        }
    }
    fn lock(&self) -> std::sync::MutexGuard<'_, HashMap<String, Option<Value>>> {
        self.held
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
    }
    fn take(&self, key: &str) -> Option<Option<Value>> {
        self.lock().remove(key)
    }
    fn forget(&self, key: &str) {
        self.lock().remove(key);
    }
}
#[async_trait]
impl Store for ReadAhead {
    async fn get(&self, key: &str) -> Result<Option<Value>> {
        match self.take(key) {
            Some(value) => Ok(value),
            None => self.store.get(key).await,
        }
    }
    async fn put(&self, key: &str, value: Value, ttl: u64) -> Result<()> {
        self.forget(key);
        self.store.put(key, value, ttl).await
    }
    async fn delete(&self, key: &str) -> Result<()> {
        self.forget(key);
        self.store.delete(key).await
    }
    async fn lease(&self, key: &str, owner: &str, ttl_ms: u64) -> Result<bool> {
        self.store.lease(key, owner, ttl_ms).await
    }
    async fn release(&self, key: &str, owner: &str) -> Result<()> {
        self.store.release(key, owner).await
    }
    async fn nonce(&self, signer: &str, now: u64) -> Result<u64> {
        self.store.nonce(signer, now).await
    }
    async fn get_many(&self, keys: &[String]) -> Vec<Result<Option<Value>>> {
        let mut values: Vec<Option<Result<Option<Value>>>> =
            keys.iter().map(|key| self.take(key).map(Ok)).collect();
        let missing: Vec<String> = keys
            .iter()
            .zip(&values)
            .filter(|(_, value)| value.is_none())
            .map(|(key, _)| key.clone())
            .collect();
        let mut read = self.store.get_many(&missing).await.into_iter();
        for value in values.iter_mut().filter(|value| value.is_none()) {
            *value = Some(read.next().unwrap_or(Err(StateError)));
        }
        values.into_iter().flatten().collect()
    }
    async fn add(&self, key: &str, by: i64, ttl: u64) -> Result<i64> {
        self.forget(key);
        self.store.add(key, by, ttl).await
    }
    async fn enqueue(&self, queue: &str, id: &str, entry: Value) -> Result<bool> {
        self.forget(queue);
        self.store.enqueue(queue, id, entry).await
    }
    async fn queue_len(&self, queue: &str) -> Result<u64> {
        self.store.queue_len(queue).await
    }
    async fn queue_front(&self, queue: &str, limit: usize) -> Result<Vec<(String, Value)>> {
        self.store.queue_front(queue, limit).await
    }
    async fn dequeue(&self, queue: &str, id: &str) -> Result<()> {
        self.forget(queue);
        self.store.dequeue(queue, id).await
    }
    async fn get_stamped(&self, key: &str) -> Result<Option<Stamped>> {
        self.forget(key);
        self.store.get_stamped(key).await
    }
    async fn delete_if(&self, key: &str, read: &Stamped) -> Result<bool> {
        self.forget(key);
        self.store.delete_if(key, read).await
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

/// A Redis stand-in for tests: the commands this crate sends, answered over
/// the real wire protocol, so the Redis store's own commands, pipelines and
/// replies run without a server. It keeps no expiry clock: it records the
/// lifetime a value was given, for the tests to check.
#[cfg(test)]
pub(crate) mod stand_in {
    use super::*;
    use std::collections::VecDeque;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};

    #[derive(Clone, Debug, PartialEq)]
    pub(crate) enum Item {
        /// A string and the lifetime, in seconds, it was last given.
        Text(Vec<u8>, Option<u64>),
        Hash(Vec<(Vec<u8>, Vec<u8>)>),
        List(VecDeque<Vec<u8>>),
    }
    type Data = Arc<std::sync::Mutex<HashMap<Vec<u8>, Item>>>;
    /// What crossed the wire since the last `take_traffic`.
    #[derive(Clone, Debug, Default)]
    pub(crate) struct Traffic {
        /// Every command, as its name and arguments.
        pub(crate) commands: Vec<Vec<String>>,
        /// How many times the server answered after reading: one per round
        /// trip of a client that waits for each answer.
        pub(crate) exchanges: usize,
        pub(crate) bytes_in: usize,
        pub(crate) bytes_out: usize,
    }
    pub(crate) struct StandIn {
        pub(crate) url: SecretString,
        data: Data,
        traffic: Arc<std::sync::Mutex<Traffic>>,
    }
    impl StandIn {
        pub(crate) async fn start() -> Self {
            let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
            let url = format!("redis://{}", listener.local_addr().unwrap()).into();
            let data: Data = Arc::default();
            let traffic: Arc<std::sync::Mutex<Traffic>> = Arc::default();
            let (shared, seen) = (data.clone(), traffic.clone());
            tokio::spawn(async move {
                while let Ok((socket, _)) = listener.accept().await {
                    tokio::spawn(serve(socket, shared.clone(), seen.clone()));
                }
            });
            Self { url, data, traffic }
        }
        pub(crate) async fn store(&self) -> RedisStore {
            RedisStore::connect(&self.url).await.unwrap()
        }
        pub(crate) fn take_traffic(&self) -> Traffic {
            std::mem::take(&mut *self.traffic.lock().unwrap())
        }
        pub(crate) fn item(&self, key: &str) -> Option<Item> {
            self.data.lock().unwrap().get(key.as_bytes()).cloned()
        }
        pub(crate) fn set_text(&self, key: &str, text: &str) {
            let item = Item::Text(text.as_bytes().to_vec(), None);
            self.data
                .lock()
                .unwrap()
                .insert(key.as_bytes().to_vec(), item);
        }
        pub(crate) fn set_list(&self, key: &str, list: VecDeque<Vec<u8>>) {
            let item = Item::List(list);
            self.data
                .lock()
                .unwrap()
                .insert(key.as_bytes().to_vec(), item);
        }
    }
    async fn serve(
        mut socket: tokio::net::TcpStream,
        data: Data,
        traffic: Arc<std::sync::Mutex<Traffic>>,
    ) {
        let mut buffer = Vec::new();
        let mut chunk = [0_u8; 65_536];
        // Commands queued by MULTI on this connection, until EXEC.
        let mut queued: Option<Vec<Vec<Vec<u8>>>> = None;
        while let Ok(read) = socket.read(&mut chunk).await {
            if read == 0 {
                return;
            }
            traffic.lock().unwrap().bytes_in += read;
            buffer.extend_from_slice(&chunk[..read]);
            let mut answer = Vec::new();
            while let Some((command, used)) = parse(&buffer) {
                buffer.drain(..used);
                let name = String::from_utf8_lossy(&command[0]).to_uppercase();
                traffic.lock().unwrap().commands.push(
                    command
                        .iter()
                        .map(|part| String::from_utf8_lossy(part).into_owned())
                        .collect(),
                );
                match (name.as_str(), queued.as_mut()) {
                    ("MULTI", None) => {
                        queued = Some(Vec::new());
                        answer.extend_from_slice(b"+OK\r\n");
                    }
                    ("EXEC", Some(_)) => {
                        let commands = queued.take().unwrap_or_default();
                        let mut store = data.lock().unwrap();
                        answer.extend_from_slice(format!("*{}\r\n", commands.len()).as_bytes());
                        for command in commands {
                            answer.extend(execute(&mut store, &command));
                        }
                    }
                    (_, Some(commands)) => {
                        commands.push(command);
                        answer.extend_from_slice(b"+QUEUED\r\n");
                    }
                    (_, None) => answer.extend(execute(&mut data.lock().unwrap(), &command)),
                }
            }
            if answer.is_empty() {
                continue;
            }
            {
                let mut seen = traffic.lock().unwrap();
                seen.exchanges += 1;
                seen.bytes_out += answer.len();
            }
            if socket.write_all(&answer).await.is_err() {
                return;
            }
        }
    }
    /// One command, as a RESP array of bulk strings, and the bytes it took.
    fn parse(buffer: &[u8]) -> Option<(Vec<Vec<u8>>, usize)> {
        fn line(buffer: &[u8], from: usize) -> Option<(usize, usize)> {
            let end = buffer[from..].windows(2).position(|w| w == b"\r\n")? + from;
            let number = std::str::from_utf8(&buffer[from + 1..end])
                .ok()?
                .parse()
                .ok()?;
            Some((number, end + 2))
        }
        if buffer.first() != Some(&b'*') {
            return None;
        }
        let (count, mut at) = line(buffer, 0)?;
        let mut parts = Vec::with_capacity(count);
        for _ in 0..count {
            let (length, start) = line(buffer, at)?;
            if buffer.len() < start + length + 2 {
                return None;
            }
            parts.push(buffer[start..start + length].to_vec());
            at = start + length + 2;
        }
        Some((parts, at))
    }
    fn bulk(value: Option<&[u8]>) -> Vec<u8> {
        match value {
            None => b"$-1\r\n".to_vec(),
            Some(value) => {
                let mut out = format!("${}\r\n", value.len()).into_bytes();
                out.extend_from_slice(value);
                out.extend_from_slice(b"\r\n");
                out
            }
        }
    }
    fn array(values: Vec<Option<Vec<u8>>>) -> Vec<u8> {
        let mut out = format!("*{}\r\n", values.len()).into_bytes();
        for value in values {
            out.extend(bulk(value.as_deref()));
        }
        out
    }
    fn integer(value: i64) -> Vec<u8> {
        format!(":{value}\r\n").into_bytes()
    }
    const WRONG_TYPE: &[u8] =
        b"-WRONGTYPE Operation against a key holding the wrong kind of value\r\n";
    fn execute(data: &mut HashMap<Vec<u8>, Item>, command: &[Vec<u8>]) -> Vec<u8> {
        let name = String::from_utf8_lossy(&command[0]).to_uppercase();
        let args = &command[1..];
        let text = |data: &HashMap<Vec<u8>, Item>, key: &[u8]| match data.get(key) {
            None => Ok(None),
            Some(Item::Text(value, _)) => Ok(Some(value.clone())),
            Some(_) => Err(()),
        };
        match name.as_str() {
            "CLIENT" => b"+OK\r\n".to_vec(),
            // The one script sent with EVAL, by its text: remove the key while
            // it holds the given value.
            "EVAL" if args[0] == DELETE_IF_UNCHANGED.as_bytes() && args[1] == b"1" => {
                match data.get(&args[2]) {
                    Some(Item::Text(value, _)) if *value == args[3] => {
                        data.remove(&args[2]);
                        integer(1)
                    }
                    Some(Item::Text(..)) | None => integer(0),
                    Some(_) => WRONG_TYPE.to_vec(),
                }
            }
            // The lifetime a value was given, not what is left of it.
            "TTL" => integer(match data.get(&args[0]) {
                None => -2,
                Some(Item::Text(_, Some(ttl))) => *ttl as i64,
                Some(_) => -1,
            }),
            "GET" => match text(data, &args[0]) {
                Ok(value) => bulk(value.as_deref()),
                Err(()) => WRONG_TYPE.to_vec(),
            },
            "MGET" => array(
                args.iter()
                    .map(|key| text(data, key).ok().flatten())
                    .collect(),
            ),
            "SET" => {
                let options: Vec<String> = args[2..]
                    .iter()
                    .map(|o| String::from_utf8_lossy(o).to_uppercase())
                    .collect();
                if options.iter().any(|o| o == "NX") && data.contains_key(&args[0]) {
                    return b"$-1\r\n".to_vec();
                }
                let ttl = options
                    .iter()
                    .position(|o| o == "EX")
                    .and_then(|i| options.get(i + 1)?.parse().ok());
                data.insert(args[0].clone(), Item::Text(args[1].clone(), ttl));
                b"+OK\r\n".to_vec()
            }
            "DEL" => integer(
                args.iter()
                    .filter(|key| data.remove(key.as_slice()).is_some())
                    .count() as i64,
            ),
            "INCRBY" => {
                let by: i64 = String::from_utf8_lossy(&args[1]).parse().unwrap();
                let (current, ttl) = match data.get(&args[0]) {
                    None => (Some(0), None),
                    Some(Item::Text(value, ttl)) => {
                        (String::from_utf8_lossy(value).parse::<i64>().ok(), *ttl)
                    }
                    Some(_) => return WRONG_TYPE.to_vec(),
                };
                let Some(current) = current else {
                    return b"-ERR value is not an integer or out of range\r\n".to_vec();
                };
                let next = current + by;
                data.insert(
                    args[0].clone(),
                    Item::Text(next.to_string().into_bytes(), ttl),
                );
                integer(next)
            }
            "HSETNX" | "HLEN" | "HMGET" | "HDEL" => {
                let mut fields = match data.get(&args[0]) {
                    None => Vec::new(),
                    Some(Item::Hash(fields)) => fields.clone(),
                    Some(_) => return WRONG_TYPE.to_vec(),
                };
                let answer = match name.as_str() {
                    "HSETNX" => {
                        let new = !fields.iter().any(|(field, _)| *field == args[1]);
                        if new {
                            fields.push((args[1].clone(), args[2].clone()));
                        }
                        integer(i64::from(new))
                    }
                    "HLEN" => integer(fields.len() as i64),
                    "HMGET" => array(
                        args[1..]
                            .iter()
                            .map(|wanted| {
                                let found = fields.iter().find(|(field, _)| field == wanted);
                                found.map(|(_, value)| value.clone())
                            })
                            .collect(),
                    ),
                    _ => {
                        let before = fields.len();
                        fields.retain(|(field, _)| !args[1..].contains(field));
                        integer((before - fields.len()) as i64)
                    }
                };
                if fields.is_empty() {
                    data.remove(&args[0]);
                } else {
                    data.insert(args[0].clone(), Item::Hash(fields));
                }
                answer
            }
            "RPUSH" | "LRANGE" | "LREM" | "LLEN" => {
                let mut list = match data.get(&args[0]) {
                    None => VecDeque::new(),
                    Some(Item::List(list)) => list.clone(),
                    Some(_) => return WRONG_TYPE.to_vec(),
                };
                let number =
                    |at: usize| -> i64 { String::from_utf8_lossy(&args[at]).parse().unwrap() };
                let answer = match name.as_str() {
                    "RPUSH" => {
                        list.extend(args[1..].iter().cloned());
                        integer(list.len() as i64)
                    }
                    "LLEN" => integer(list.len() as i64),
                    "LRANGE" => {
                        let length = list.len() as i64;
                        let index = |i: i64| if i < 0 { length + i } else { i };
                        let (start, stop) =
                            (index(number(1)).max(0), index(number(2)).min(length - 1));
                        let values = (start..=stop).filter_map(|i| list.get(i as usize).cloned());
                        array(values.map(Some).collect())
                    }
                    _ => {
                        // LREM with a count of zero removes every match;
                        // with a positive count, that many from the head.
                        let count = number(1);
                        assert!(count >= 0, "the stand-in removes from the head only");
                        let mut removed = 0;
                        list.retain(|value| {
                            let remove = *value == args[2] && (count == 0 || removed < count);
                            removed += i64::from(remove);
                            !remove
                        });
                        integer(removed)
                    }
                };
                if list.is_empty() {
                    data.remove(&args[0]);
                } else {
                    data.insert(args[0].clone(), Item::List(list));
                }
                answer
            }
            _ => format!("-ERR unknown command '{name}'\r\n").into_bytes(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::{stand_in::Item, stand_in::StandIn, *};
    use serde_json::json;
    use std::collections::VecDeque;

    /// What every store must do with the batched reads, the counter and the
    /// queues, whatever keeps them. Keys start with `prefix`.
    async fn contract(store: Arc<dyn Store>, prefix: &str) {
        let key = |name: &str| format!("{prefix}{name}");
        // Several values at once, in the order asked, absent ones as such.
        store.put(&key("a"), json!({"n":1}), 0).await.unwrap();
        store.put(&key("b"), json!("two"), 60).await.unwrap();
        let read = store
            .get_many(&[key("b"), key("missing"), key("a"), key("b")])
            .await;
        let read: Vec<Option<Value>> = read.into_iter().map(Result::unwrap).collect();
        assert_eq!(
            read,
            [
                Some(json!("two")),
                None,
                Some(json!({"n":1})),
                Some(json!("two"))
            ]
        );
        assert!(store.get_many(&[]).await.is_empty());

        // The counter: every caller gets a sum of its own, none is lost.
        let counter = key("counter");
        assert_eq!(store.add(&counter, 1, 86_400).await.unwrap(), 1);
        assert_eq!(store.add(&counter, -1, 86_400).await.unwrap(), 0);
        let adds = (0..40).map(|_| {
            let (store, counter) = (store.clone(), counter.clone());
            tokio::spawn(async move { store.add(&counter, 1, 86_400).await.unwrap() })
        });
        let mut sums = Vec::new();
        for add in adds {
            sums.push(add.await.unwrap());
        }
        sums.sort_unstable();
        assert_eq!(sums, (1..=40).collect::<Vec<i64>>());
        assert_eq!(store.get(&counter).await.unwrap(), Some(json!(40)));
        // A count written as a plain value, as the previous release did.
        store.put(&key("old"), json!(3), 86_400).await.unwrap();
        assert_eq!(store.add(&key("old"), 1, 86_400).await.unwrap(), 4);
        assert_eq!(store.get(&key("old")).await.unwrap(), Some(json!(4)));
        // Something that is not a whole number is not counted on.
        assert!(store.add(&key("b"), 1, 86_400).await.is_err());

        // A queue keeps the order entries were added in, one per id.
        let queue = key("queue");
        assert_eq!(store.queue_len(&queue).await.unwrap(), 0);
        assert!(store.queue_front(&queue, 10).await.unwrap().is_empty());
        for n in 1..=5 {
            let id = format!("id-{n}");
            let added = store.enqueue(&queue, &id, json!({"n":n})).await;
            assert!(added.unwrap());
        }
        assert!(
            !store
                .enqueue(&queue, "id-2", json!({"n":99}))
                .await
                .unwrap()
        );
        assert_eq!(store.queue_len(&queue).await.unwrap(), 5);
        let front = store.queue_front(&queue, 3).await.unwrap();
        assert_eq!(
            front,
            [
                ("id-1".to_owned(), json!({"n":1})),
                ("id-2".to_owned(), json!({"n":2})),
                ("id-3".to_owned(), json!({"n":3}))
            ]
        );
        // Removal takes that entry only, wherever it is; an unknown id is
        // no change.
        store.dequeue(&queue, "id-2").await.unwrap();
        store.dequeue(&queue, "id-1").await.unwrap();
        store.dequeue(&queue, "unknown").await.unwrap();
        let ids = |front: Vec<(String, Value)>| -> Vec<String> {
            front.into_iter().map(|(id, _)| id).collect()
        };
        assert_eq!(
            ids(store.queue_front(&queue, 10).await.unwrap()),
            ["id-3", "id-4", "id-5"]
        );
        assert_eq!(store.queue_len(&queue).await.unwrap(), 3);
        // An id that was removed can be added again; it goes to the back.
        assert!(store.enqueue(&queue, "id-1", json!({"n":1})).await.unwrap());
        assert_eq!(
            ids(store.queue_front(&queue, 10).await.unwrap()),
            ["id-3", "id-4", "id-5", "id-1"]
        );
        for id in ["id-3", "id-4", "id-5", "id-1"] {
            store.dequeue(&queue, id).await.unwrap();
        }
        assert_eq!(store.queue_len(&queue).await.unwrap(), 0);
        assert!(store.queue_front(&queue, 10).await.unwrap().is_empty());

        // A key is removed only while it holds what was read.
        let list = key("list");
        store.put(&list, json!([1, 2]), 0).await.unwrap();
        let read = store.get_stamped(&list).await.unwrap().unwrap();
        assert_eq!(read.value, json!([1, 2]));
        store.put(&list, json!([1, 2, 3]), 0).await.unwrap();
        assert!(!store.delete_if(&list, &read).await.unwrap());
        assert_eq!(store.get(&list).await.unwrap(), Some(json!([1, 2, 3])));
        let read = store.get_stamped(&list).await.unwrap().unwrap();
        assert!(store.delete_if(&list, &read).await.unwrap());
        assert_eq!(store.get(&list).await.unwrap(), None);
        assert!(!store.delete_if(&list, &read).await.unwrap());
        assert!(store.get_stamped(&list).await.unwrap().is_none());
        for name in ["a", "b", "counter", "old"] {
            store.delete(&key(name)).await.unwrap();
        }
    }

    #[tokio::test]
    async fn a_conditional_removal_compares_the_text_redis_holds() {
        let redis = StandIn::start().await;
        let store = redis.store().await;
        // Written by something other than this crate, with spaces in it.
        redis.set_text("bot:kv:spaced", "[1, 2]");
        let read = store.get_stamped("spaced").await.unwrap().unwrap();
        assert_eq!(read.value, json!([1, 2]));
        redis.take_traffic();
        assert!(store.delete_if("spaced", &read).await.unwrap());
        let traffic = redis.take_traffic();
        assert_eq!(traffic.exchanges, 1);
        assert_eq!(traffic.commands[0][3], "bot:kv:spaced");
        assert_eq!(traffic.commands[0][4], "[1, 2]");
        assert_eq!(redis.item("bot:kv:spaced"), None);
    }

    #[tokio::test]
    async fn the_in_memory_store_keeps_the_contract() {
        contract(Arc::new(MemoryStore::default()), "").await;
    }

    /// Against a real Redis, only when `REDIS_TEST_URL` names a disposable one
    /// on this machine. Every key it touches starts with a fresh prefix.
    #[tokio::test]
    async fn a_real_redis_keeps_the_contract() {
        let Ok(url) = std::env::var("REDIS_TEST_URL") else {
            return;
        };
        let parsed = reqwest::Url::parse(&url).unwrap();
        assert!(matches!(
            parsed.host_str(),
            Some("127.0.0.1" | "localhost" | "[::1]")
        ));
        redis_checks(url.into()).await;
    }

    /// The checks the real Redis runs, on the stand-in: they hold there too.
    #[tokio::test]
    async fn the_real_redis_checks_hold_on_the_stand_in() {
        let redis = StandIn::start().await;
        redis_checks(redis.url.clone()).await;
    }

    /// The contract and the expiry of counts, through the Redis store at
    /// `url`.
    async fn redis_checks(url: SecretString) {
        let store = Arc::new(RedisStore::connect(&url).await.unwrap());
        let prefix = format!("contract-{}:", uuid::Uuid::new_v4().simple());
        contract(store.clone(), &prefix).await;
        // A count is created with its expiry; an existing one keeps its own.
        let client = redis::Client::open(url.expose_secret()).unwrap();
        let mut raw = client.get_multiplexed_async_connection().await.unwrap();
        let mut ttl = async |key: &str| -> i64 {
            redis::cmd("TTL")
                .arg(format!("bot:kv:{prefix}{key}"))
                .query_async(&mut raw)
                .await
                .unwrap()
        };
        store
            .add(&format!("{prefix}count"), 1, 86_400)
            .await
            .unwrap();
        assert!((86_000..=86_400).contains(&ttl("count").await));
        store
            .put(&format!("{prefix}kept"), serde_json::json!(7), 600)
            .await
            .unwrap();
        assert_eq!(
            store
                .add(&format!("{prefix}kept"), 1, 86_400)
                .await
                .unwrap(),
            8
        );
        assert!((500..=600).contains(&ttl("kept").await));
        for key in ["count", "kept"] {
            store.delete(&format!("{prefix}{key}")).await.unwrap();
        }
    }

    #[tokio::test]
    async fn several_values_are_read_in_one_round_trip_and_fail_one_by_one() {
        let redis = StandIn::start().await;
        let store = redis.store().await;
        store.put("a", json!(1), 0).await.unwrap();
        redis.set_text("bot:kv:broken", "not json");
        redis.take_traffic();
        let keys = ["a", "broken", "missing"].map(String::from);
        let read = store.get_many(&keys).await;
        assert_eq!(read[0].as_ref().unwrap(), &Some(json!(1)));
        assert!(read[1].is_err());
        assert_eq!(read[2].as_ref().unwrap(), &None);
        let traffic = redis.take_traffic();
        assert_eq!(traffic.exchanges, 1);
        assert_eq!(
            traffic.commands,
            [vec!["MGET", "bot:kv:a", "bot:kv:broken", "bot:kv:missing"]]
        );
    }

    #[tokio::test]
    async fn a_count_is_created_with_its_expiry_in_the_same_step() {
        let redis = StandIn::start().await;
        let store = redis.store().await;
        redis.take_traffic();
        assert_eq!(store.add("count", 1, 86_400).await.unwrap(), 1);
        let traffic = redis.take_traffic();
        assert_eq!(traffic.exchanges, 1);
        assert_eq!(
            traffic.commands,
            [
                vec!["MULTI"],
                vec!["SET", "bot:kv:count", "0", "EX", "86400", "NX"],
                vec!["INCRBY", "bot:kv:count", "1"],
                vec!["EXEC"]
            ]
        );
        assert_eq!(
            redis.item("bot:kv:count"),
            Some(Item::Text(b"1".to_vec(), Some(86_400)))
        );
        // An existing count keeps its expiry: adding never extends it.
        store.put("old", json!(7), 600).await.unwrap();
        assert_eq!(store.add("old", 1, 86_400).await.unwrap(), 8);
        assert_eq!(
            redis.item("bot:kv:old"),
            Some(Item::Text(b"8".to_vec(), Some(600)))
        );
    }

    #[tokio::test]
    async fn a_queue_entry_costs_the_same_however_many_wait() {
        let redis = StandIn::start().await;
        let store = redis.store().await;
        let entry = json!({"entry":"x".repeat(200)});
        let mut sent = Vec::new();
        for n in 0..200 {
            redis.take_traffic();
            store
                .enqueue("q", &format!("id-{n:04}"), entry.clone())
                .await
                .unwrap();
            let traffic = redis.take_traffic();
            assert_eq!(traffic.exchanges, 1);
            sent.push(traffic.bytes_in);
        }
        // Adding the 200th sends what adding the first did.
        assert_eq!(sent.first(), sent.last());
        let mut received = Vec::new();
        for n in 0..200 {
            redis.take_traffic();
            let front = store.queue_front("q", 1).await.unwrap();
            assert_eq!(front[0].0, format!("id-{n:04}"));
            store.dequeue("q", &front[0].0).await.unwrap();
            let traffic = redis.take_traffic();
            assert_eq!(traffic.exchanges, 3);
            received.push(traffic.bytes_out + traffic.bytes_in);
        }
        assert_eq!(received.first(), received.last());
        assert_eq!(redis.item("bot:queue:q:entries"), None);
        assert_eq!(redis.item("bot:queue:q:order"), None);
    }

    #[tokio::test]
    async fn an_id_added_twice_while_waiting_is_read_once_and_removed_whole() {
        let redis = StandIn::start().await;
        let store = redis.store().await;
        assert!(store.enqueue("q", "a", json!(1)).await.unwrap());
        assert!(store.enqueue("q", "b", json!(2)).await.unwrap());
        assert!(!store.enqueue("q", "a", json!(3)).await.unwrap());
        let front = store.queue_front("q", 10).await.unwrap();
        assert_eq!(
            front,
            [("a".to_owned(), json!(1)), ("b".to_owned(), json!(2))]
        );
        store.dequeue("q", "a").await.unwrap();
        // Both places in line of the id went with its entry.
        let order = VecDeque::from([b"b".to_vec()]);
        assert_eq!(redis.item("bot:queue:q:order"), Some(Item::List(order)));
        // A place in line whose entry is missing is passed over, even when
        // it fills a whole read.
        let stale = VecDeque::from(["x", "y", "b", "z"].map(|id| id.as_bytes().to_vec()));
        redis.set_list("bot:queue:q:order", stale);
        let front = store.queue_front("q", 2).await.unwrap();
        assert_eq!(front, [("b".to_owned(), json!(2))]);
    }

    #[tokio::test]
    async fn a_value_read_ahead_is_handed_out_once_and_never_after_a_write() {
        let inner = Arc::new(MemoryStore::default());
        inner.put("a", json!(1), 0).await.unwrap();
        inner.put("b", json!(2), 0).await.unwrap();
        let ahead = ReadAhead::new(inner.clone());
        ahead.fetch(&["a".into(), "b".into(), "c".into()]).await;
        // Changed behind the read-ahead: the first read gets what was read
        // ahead, the next one what the store holds.
        inner.put("a", json!(10), 0).await.unwrap();
        assert_eq!(ahead.get("a").await.unwrap(), Some(json!(1)));
        assert_eq!(ahead.get("a").await.unwrap(), Some(json!(10)));
        // A write through it forgets what was read ahead.
        ahead.put("b", json!(20), 0).await.unwrap();
        assert_eq!(ahead.get("b").await.unwrap(), Some(json!(20)));
        // A key read ahead as absent stays absent for its first read only.
        inner.put("c", json!(3), 0).await.unwrap();
        assert_eq!(ahead.get("c").await.unwrap(), None);
        assert_eq!(ahead.get("c").await.unwrap(), Some(json!(3)));
        // Removing and counting forget too.
        ahead.fetch(&["a".into(), "c".into()]).await;
        ahead.delete("a").await.unwrap();
        assert_eq!(ahead.get("a").await.unwrap(), None);
        inner.put("c", json!(5), 0).await.unwrap();
        ahead.fetch(&["c".into()]).await;
        assert_eq!(ahead.add("c", 1, 0).await.unwrap(), 6);
        assert_eq!(ahead.get("c").await.unwrap(), Some(json!(6)));
        // Batched reads take what is held and read the rest.
        inner.put("d", json!(4), 0).await.unwrap();
        ahead.fetch(&["d".into()]).await;
        inner.put("d", json!(40), 0).await.unwrap();
        let read: Vec<Option<Value>> = ahead
            .get_many(&["c".into(), "d".into(), "d".into()])
            .await
            .into_iter()
            .map(Result::unwrap)
            .collect();
        assert_eq!(read, [Some(json!(6)), Some(json!(4)), Some(json!(40))]);
    }

    #[tokio::test]
    async fn a_value_that_could_not_be_read_ahead_is_read_where_it_is_used() {
        let redis = StandIn::start().await;
        let store: Arc<dyn Store> = Arc::new(redis.store().await);
        redis.set_text("bot:kv:broken", "not json");
        store.put("fine", json!(1), 0).await.unwrap();
        let ahead = ReadAhead::new(store);
        ahead.fetch(&["broken".into(), "fine".into()]).await;
        redis.take_traffic();
        assert_eq!(ahead.get("fine").await.unwrap(), Some(json!(1)));
        assert!(redis.take_traffic().commands.is_empty());
        // The unreadable one fails where it is read, as it always did.
        assert!(ahead.get("broken").await.is_err());
        assert_eq!(
            redis.take_traffic().commands,
            [vec!["GET", "bot:kv:broken"]]
        );
    }
}

#[cfg(test)]
mod timeout_tests {
    use super::*;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};

    /// A server that accepts connections and optionally answers the client's
    /// connection handshake, but never answers a data command.
    async fn unresponsive_server(handshake: bool) -> SecretString {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let url = format!("redis://{}", listener.local_addr().unwrap());
        tokio::spawn(async move {
            loop {
                let (mut socket, _) = listener.accept().await.unwrap();
                tokio::spawn(async move {
                    let mut buffer = [0_u8; 4096];
                    while let Ok(read) = socket.read(&mut buffer).await {
                        if read == 0 {
                            break;
                        }
                        let text = String::from_utf8_lossy(&buffer[..read]).to_uppercase();
                        if handshake {
                            for _ in 0..text.matches("CLIENT").count() {
                                let _ = socket.write_all(b"+OK\r\n").await;
                            }
                        }
                    }
                });
            }
        });
        url.into()
    }
    fn short() -> ConnectionManagerConfig {
        let limit = Duration::from_millis(200);
        connection_config(limit, limit).set_number_of_retries(0)
    }

    #[tokio::test]
    async fn connecting_to_a_silent_server_fails_instead_of_hanging() {
        let url = unresponsive_server(false).await;
        let started = std::time::Instant::now();
        assert!(RedisStore::connect_with(&url, short()).await.is_err());
        assert!(started.elapsed() < Duration::from_secs(3));
    }

    #[tokio::test]
    async fn a_command_that_is_never_answered_fails_instead_of_hanging() {
        let url = unresponsive_server(true).await;
        let store = RedisStore::connect_with(&url, short()).await.unwrap();
        let started = std::time::Instant::now();
        assert!(store.get("anything").await.is_err());
        assert!(store.lease("worker", "owner", 60_000).await.is_err());
        assert!(started.elapsed() < Duration::from_secs(3));
    }
}

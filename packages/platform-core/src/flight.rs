//! Request coalescing: callers that ask for the same thing at the same moment
//! share one piece of work instead of each starting their own.
//!
//! [`Group`] is the async form, [`Blocking`] the same for synchronous code on
//! blocking threads, and [`Cached`] adds a short lifetime to a successful
//! result. None of them hold a lock while work runs.
use std::{
    collections::HashMap,
    future::Future,
    hash::Hash,
    sync::{Arc, Mutex, MutexGuard, OnceLock, PoisonError},
    time::Duration,
};
use tokio::{sync::OnceCell, time::Instant};

/// The maps below are only locked for a lookup, an insert or a removal, so a
/// poisoned lock still guards a consistent map.
fn locked<T>(mutex: &Mutex<T>) -> MutexGuard<'_, T> {
    mutex.lock().unwrap_or_else(PoisonError::into_inner)
}

/// What a cell reports about itself to the code that tidies the map.
trait Slot: Default {
    fn done(&self) -> bool;
}
impl<V> Slot for OnceCell<V> {
    fn done(&self) -> bool {
        self.initialized()
    }
}
impl<V> Slot for OnceLock<V> {
    fn done(&self) -> bool {
        self.get().is_some()
    }
}

type Calls<K, C> = Mutex<HashMap<K, Arc<C>>>;

/// Takes a caller out of a call when it returns or is cancelled. The entry is
/// removed once the call has a result, so the next caller starts fresh, or
/// when the last caller gives up, so an abandoned key does not stay behind.
///
/// The caller's own reference to the cell lives here and is given up while
/// the map is locked. Counting references is only exact that way: two callers
/// leaving at the same moment are seen one after the other, and the second
/// one finds itself the last.
struct Leave<'a, K: Eq + Hash, C: Slot> {
    calls: &'a Calls<K, C>,
    key: &'a K,
    cell: Option<Arc<C>>,
}
impl<'a, K: Eq + Hash + Clone, C: Slot> Leave<'a, K, C> {
    fn join(calls: &'a Calls<K, C>, key: &'a K) -> Self {
        let cell = locked(calls).entry(key.clone()).or_default().clone();
        Self {
            calls,
            key,
            cell: Some(cell),
        }
    }
}
impl<K: Eq + Hash, C: Slot> Leave<'_, K, C> {
    fn cell(&self) -> &C {
        self.cell.as_deref().expect("held until the caller leaves")
    }
}
impl<K: Eq + Hash, C: Slot> Drop for Leave<'_, K, C> {
    fn drop(&mut self) {
        let mut calls = locked(self.calls);
        let Some(cell) = self.cell.take() else { return };
        let mine = calls
            .get(self.key)
            .is_some_and(|current| Arc::ptr_eq(current, &cell));
        // Two references are the map's and this caller's: nobody else waits.
        if mine && (cell.done() || Arc::strong_count(&cell) <= 2) {
            calls.remove(self.key);
        }
        drop(cell);
    }
}

/// Coalesces concurrent async calls by key. Nothing is kept once a call has
/// finished; a failure is shared with the callers that were waiting for it and
/// then forgotten.
pub struct Group<K, V> {
    calls: Calls<K, OnceCell<V>>,
}
impl<K, V> Default for Group<K, V> {
    fn default() -> Self {
        Self {
            calls: Mutex::new(HashMap::new()),
        }
    }
}
impl<K: Eq + Hash + Clone, V: Clone> Group<K, V> {
    pub fn new() -> Self {
        Self::default()
    }
    /// Runs `work` unless a call for `key` is already in flight, in which case
    /// this caller waits for that call's result. If the caller doing the work
    /// is cancelled, one of the waiting callers runs its own `work`.
    pub async fn run<F, Fut>(&self, key: K, work: F) -> V
    where
        F: FnOnce() -> Fut,
        Fut: Future<Output = V>,
    {
        let call = Leave::join(&self.calls, &key);
        call.cell().get_or_init(work).await.clone()
    }
    #[cfg(test)]
    fn in_flight(&self) -> usize {
        locked(&self.calls).len()
    }
}

/// [`Group`] for synchronous work on blocking threads: a second thread asking
/// for the same key blocks until the first one has the result.
pub struct Blocking<K, V> {
    calls: Calls<K, OnceLock<V>>,
}
impl<K, V> Default for Blocking<K, V> {
    fn default() -> Self {
        Self {
            calls: Mutex::new(HashMap::new()),
        }
    }
}
impl<K: Eq + Hash + Clone, V: Clone> Blocking<K, V> {
    pub fn new() -> Self {
        Self::default()
    }
    pub fn run(&self, key: K, work: impl FnOnce() -> V) -> V {
        let call = Leave::join(&self.calls, &key);
        call.cell().get_or_init(work).clone()
    }
}

/// A successful result is reused for `ttl`; a miss is coalesced, so an expired
/// or absent key costs one piece of work however many callers want it. Errors
/// are never stored.
pub struct Cached<K, V, E> {
    ttl: Duration,
    capacity: usize,
    values: Mutex<HashMap<K, (Instant, V)>>,
    flights: Group<K, Result<V, E>>,
}
impl<K: Eq + Hash + Clone, V: Clone, E: Clone> Cached<K, V, E> {
    pub fn new(ttl: Duration, capacity: usize) -> Self {
        Self {
            ttl,
            capacity: capacity.max(1),
            values: Mutex::new(HashMap::new()),
            flights: Group::new(),
        }
    }
    fn fresh(&self, key: &K) -> Option<V> {
        locked(&self.values)
            .get(key)
            .filter(|(stored, _)| stored.elapsed() < self.ttl)
            .map(|(_, value)| value.clone())
    }
    fn keep(&self, key: K, value: V) {
        let mut values = locked(&self.values);
        if values.len() >= self.capacity && !values.contains_key(&key) {
            values.retain(|_, (stored, _)| stored.elapsed() < self.ttl);
            if values.len() >= self.capacity {
                values.clear();
            }
        }
        values.insert(key, (Instant::now(), value));
    }
    pub async fn get<F, Fut>(&self, key: K, work: F) -> Result<V, E>
    where
        F: FnOnce() -> Fut,
        Fut: Future<Output = Result<V, E>>,
    {
        if let Some(value) = self.fresh(&key) {
            return Ok(value);
        }
        self.flights
            .run(key.clone(), || async {
                // A caller that arrived just as a flight landed finds its result.
                if let Some(value) = self.fresh(&key) {
                    return Ok(value);
                }
                let value = work().await?;
                self.keep(key.clone(), value.clone());
                Ok(value)
            })
            .await
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use tokio::sync::Notify;

    #[tokio::test]
    async fn concurrent_callers_share_one_run_and_the_next_caller_starts_fresh() {
        let group = Arc::new(Group::<&str, usize>::new());
        let runs = Arc::new(AtomicUsize::new(0));
        let release = Arc::new(Notify::new());
        let mut callers = Vec::new();
        for _ in 0..20 {
            let (group, runs, release) = (group.clone(), runs.clone(), release.clone());
            callers.push(tokio::spawn(async move {
                group
                    .run("key", || async {
                        let run = runs.fetch_add(1, Ordering::SeqCst) + 1;
                        release.notified().await;
                        run
                    })
                    .await
            }));
        }
        while runs.load(Ordering::SeqCst) == 0 {
            tokio::task::yield_now().await;
        }
        for _ in 0..50 {
            tokio::task::yield_now().await;
        }
        release.notify_one();
        for caller in callers {
            assert_eq!(caller.await.unwrap(), 1);
        }
        assert_eq!(runs.load(Ordering::SeqCst), 1);
        assert_eq!(group.in_flight(), 0);
        assert_eq!(group.run("key", || async { 7 }).await, 7);
    }

    #[tokio::test]
    async fn different_keys_do_not_wait_for_each_other() {
        let group = Group::<u8, u8>::new();
        let blocked = group.run(1, std::future::pending::<u8>);
        tokio::pin!(blocked);
        tokio::select! {
            _ = &mut blocked => unreachable!(),
            value = group.run(2, || async { 2 }) => assert_eq!(value, 2),
        }
    }

    #[tokio::test]
    async fn a_cancelled_caller_hands_the_work_to_a_waiting_one_and_leaves_nothing_behind() {
        let group = Arc::new(Group::<&str, &str>::new());
        let started = Arc::new(Notify::new());
        let first = {
            let (group, started) = (group.clone(), started.clone());
            tokio::spawn(async move {
                group
                    .run("key", || async {
                        started.notify_one();
                        std::future::pending::<&str>().await
                    })
                    .await
            })
        };
        started.notified().await;
        let second = {
            let group = group.clone();
            tokio::spawn(async move { group.run("key", || async { "second" }).await })
        };
        for _ in 0..50 {
            tokio::task::yield_now().await;
        }
        first.abort();
        assert_eq!(second.await.unwrap(), "second");
        assert_eq!(group.in_flight(), 0);

        // A lone caller that is cancelled removes its own entry.
        let lone = {
            let group = group.clone();
            tokio::spawn(async move { group.run("gone", std::future::pending::<&str>).await })
        };
        for _ in 0..50 {
            tokio::task::yield_now().await;
        }
        assert_eq!(group.in_flight(), 1);
        lone.abort();
        let _ = lone.await;
        assert_eq!(group.in_flight(), 0);
    }

    /// Callers that are cancelled on several threads at the same moment must
    /// still leave the map empty: each gives up its reference under the lock.
    #[tokio::test(flavor = "multi_thread", worker_threads = 4)]
    async fn callers_cancelled_together_on_several_threads_leave_nothing_behind() {
        let group = Arc::new(Group::<u32, u32>::new());
        for round in 0..200 {
            let waiting: Vec<_> = (0..4)
                .map(|_| {
                    let group = group.clone();
                    tokio::spawn(async move { group.run(round, std::future::pending::<u32>).await })
                })
                .collect();
            tokio::time::sleep(Duration::from_micros(200)).await;
            for caller in &waiting {
                caller.abort();
            }
            for caller in waiting {
                let _ = caller.await;
            }
        }
        assert_eq!(group.in_flight(), 0);
    }

    #[tokio::test(start_paused = true)]
    async fn a_result_is_reused_until_it_expires_and_a_failure_is_never_kept() {
        let cache = Cached::<&str, usize, &str>::new(Duration::from_secs(30), 8);
        let runs = AtomicUsize::new(0);
        let work = || async { Ok::<_, &str>(runs.fetch_add(1, Ordering::SeqCst) + 1) };
        assert_eq!(cache.get("key", work).await, Ok(1));
        assert_eq!(cache.get("key", work).await, Ok(1));
        tokio::time::advance(Duration::from_secs(31)).await;
        assert_eq!(cache.get("key", work).await, Ok(2));

        assert_eq!(
            cache.get("bad", || async { Err::<usize, _>("down") }).await,
            Err("down")
        );
        assert_eq!(cache.get("bad", || async { Ok(5) }).await, Ok(5));
    }

    #[tokio::test(start_paused = true)]
    async fn the_cache_stays_within_its_capacity() {
        let cache = Cached::<u32, u32, ()>::new(Duration::from_secs(30), 4);
        for key in 0..40 {
            assert_eq!(cache.get(key, || async { Ok(key) }).await, Ok(key));
        }
        assert!(locked(&cache.values).len() <= 4);
    }

    #[test]
    fn blocking_threads_share_one_run() {
        let group = Arc::new(Blocking::<&str, usize>::new());
        let runs = Arc::new(AtomicUsize::new(0));
        let gate = Arc::new(std::sync::Barrier::new(8));
        let threads: Vec<_> = (0..8)
            .map(|_| {
                let (group, runs, gate) = (group.clone(), runs.clone(), gate.clone());
                std::thread::spawn(move || {
                    gate.wait();
                    group.run("key", || {
                        std::thread::sleep(Duration::from_millis(100));
                        runs.fetch_add(1, Ordering::SeqCst) + 1
                    })
                })
            })
            .collect();
        for thread in threads {
            assert_eq!(thread.join().unwrap(), 1);
        }
        assert_eq!(runs.load(Ordering::SeqCst), 1);
        assert!(locked(&group.calls).is_empty());
    }
}

use astradia_platform_core::state::{Lease, MemoryStore, Store, read, write};
use std::sync::Arc;

#[tokio::test]
async fn lease_is_exclusive_until_released_and_owner_checked() {
    let store: Arc<dyn Store> = Arc::new(MemoryStore::default());
    let first = Lease::acquire(store.clone(), "job:1".into(), 5_000)
        .await
        .unwrap()
        .expect("free lease is granted");
    assert!(
        Lease::acquire(store.clone(), "job:1".into(), 5_000)
            .await
            .unwrap()
            .is_none()
    );
    // A different key is independent, and a stranger cannot release the first.
    assert!(store.lease("job:2", "other", 5_000).await.unwrap());
    store.release("job:1", "other").await.unwrap();
    assert!(!store.lease("job:1", "other", 5_000).await.unwrap());
    first.release().await.unwrap();
    assert!(store.lease("job:1", "other", 5_000).await.unwrap());
}

#[tokio::test]
async fn same_owner_renews_and_expired_lease_can_be_taken() {
    let store = MemoryStore::default();
    assert!(store.lease("task", "a", 20).await.unwrap());
    assert!(store.lease("task", "a", 20).await.unwrap());
    assert!(!store.lease("task", "b", 20).await.unwrap());
    tokio::time::sleep(std::time::Duration::from_millis(40)).await;
    assert!(store.lease("task", "b", 20).await.unwrap());
}

#[tokio::test]
async fn nonce_never_repeats_when_the_clock_stalls_or_goes_back() {
    let store = MemoryStore::default();
    assert_eq!(store.nonce("signer", 1_000).await.unwrap(), 1_000);
    assert_eq!(store.nonce("signer", 1_000).await.unwrap(), 1_001);
    assert_eq!(store.nonce("signer", 900).await.unwrap(), 1_002);
    assert_eq!(store.nonce("signer", 5_000).await.unwrap(), 5_000);
    assert_eq!(store.nonce("other", 1).await.unwrap(), 1);
}

#[tokio::test]
async fn typed_values_round_trip_and_wrong_shapes_fail() {
    let store = MemoryStore::default();
    write(&store, "settings", &vec![1u32, 2, 3], 0)
        .await
        .unwrap();
    assert_eq!(
        read::<Vec<u32>>(&store, "settings").await.unwrap(),
        Some(vec![1, 2, 3])
    );
    assert!(read::<String>(&store, "settings").await.is_err());
    store.delete("settings").await.unwrap();
    assert_eq!(read::<Vec<u32>>(&store, "settings").await.unwrap(), None);
}

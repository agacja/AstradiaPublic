use astradia_platform_core::signing::{self, LocalSigner, Signer};
use ethers_core::types::Address;
use serde_json::Value;
#[tokio::test]
async fn matches_all_existing_sdk_golden_vectors() {
    let vectors: Vec<Value> =
        serde_json::from_str(include_str!("fixtures/hl-signing-vectors.json")).unwrap();
    assert!(!vectors.is_empty());
    for v in vectors {
        let a = &v["action"];
        let user = a["type"].as_str() == Some("usdClassTransfer");
        let hash = if user {
            signing::user_digest(a).unwrap()
        } else {
            signing::action_hash(
                a,
                v["vaultAddress"]
                    .as_str()
                    .filter(|s| !s.is_empty())
                    .map(|s| s.parse::<Address>().unwrap()),
                v["nonce"].as_u64().unwrap(),
                None,
            )
            .unwrap()
        };
        assert_eq!(hex::encode(hash), v["actionHashHex"], "{}", v["name"]);
        let digest = if user {
            hash
        } else {
            signing::l1_digest(hash, v["isTestnet"].as_bool().unwrap())
        };
        let signer =
            LocalSigner::new(&v["signerKeyHex"].as_str().unwrap().to_string().into()).unwrap();
        let sig = signer.sign_digest(digest).await.unwrap();
        assert_eq!(format!("{:064x}", sig.r), v["r"], "{}", v["name"]);
        assert_eq!(format!("{:064x}", sig.s), v["s"], "{}", v["name"]);
        assert_eq!(sig.v, v["v"].as_u64().unwrap());
        signing::verify_signature(&sig, digest, signer.address()).unwrap();
    }
}

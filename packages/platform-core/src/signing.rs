//! Hyperliquid SDK wire types and the two ways a digest gets signed: a key
//! held here, or a wallet held by the provider (Privy).
//! Keep the exact named MessagePack field order and canonical decimal strings.
use crate::http::shared_http_client;
use async_trait::async_trait;
use ethers_core::{
    types::{
        Address, H256, Signature,
        transaction::eip712::{Eip712, TypedData},
    },
    utils::keccak256,
};
use ethers_signers::{LocalWallet, Signer as EthersSigner};
use secrecy::{ExposeSecret, SecretString};
use serde::Serialize;
use serde_json::{Value, json};
use std::str::FromStr;

#[derive(Debug, thiserror::Error)]
pub enum SigningError {
    #[error("Invalid signing credential")]
    Credential,
    #[error("Invalid signing payload")]
    Payload,
    #[error("Signing service unavailable")]
    Unavailable,
    #[error("Signing service rejected request")]
    Rejected,
    #[error("Signature did not match configured wallet")]
    WrongSigner,
}
pub type Result<T> = std::result::Result<T, SigningError>;
#[async_trait]
pub trait Signer: Send + Sync {
    fn address(&self) -> Address;
    async fn sign_digest(&self, digest: [u8; 32]) -> Result<Signature>;
}
pub struct LocalSigner {
    wallet: LocalWallet,
}
impl LocalSigner {
    pub fn new(key: &SecretString) -> Result<Self> {
        Ok(Self {
            wallet: key
                .expose_secret()
                .trim()
                .parse()
                .map_err(|_| SigningError::Credential)?,
        })
    }
}
#[async_trait]
impl Signer for LocalSigner {
    fn address(&self) -> Address {
        self.wallet.address()
    }
    async fn sign_digest(&self, digest: [u8; 32]) -> Result<Signature> {
        self.wallet
            .sign_hash(H256(digest))
            .map_err(|_| SigningError::Unavailable)
    }
}
pub fn action_hash(
    action: &impl Serialize,
    vault: Option<Address>,
    nonce: u64,
    expires_after: Option<u64>,
) -> Result<[u8; 32]> {
    let mut bytes = rmp_serde::to_vec_named(action).map_err(|_| SigningError::Payload)?;
    bytes.extend_from_slice(&nonce.to_be_bytes());
    if let Some(addr) = vault {
        bytes.push(1);
        bytes.extend_from_slice(addr.as_bytes());
    } else {
        bytes.push(0);
    }
    if let Some(expiry) = expires_after {
        bytes.push(0);
        bytes.extend_from_slice(&expiry.to_be_bytes());
    }
    Ok(keccak256(bytes))
}
pub fn l1_digest(hash: [u8; 32], testnet: bool) -> [u8; 32] {
    let domain = hex::decode("d79297fcdf2ffcd4ae223d01edaa2ba214ff8f401d7c9300d995d17c82aa4040")
        .expect("constant hex");
    let mut message = keccak256("Agent(string source,bytes32 connectionId)").to_vec();
    message.extend_from_slice(&keccak256(if testnet { "b" } else { "a" }));
    message.extend_from_slice(&hash);
    let mut eip = vec![0x19, 0x01];
    eip.extend_from_slice(&domain);
    eip.extend_from_slice(&keccak256(message));
    keccak256(eip)
}
pub fn user_typed_data(action: &Value) -> Result<TypedData> {
    let (primary, fields) = match action["type"].as_str() {
        Some("usdClassTransfer") => (
            "UsdClassTransfer",
            vec![
                ("hyperliquidChain", "string"),
                ("amount", "string"),
                ("toPerp", "bool"),
                ("nonce", "uint64"),
            ],
        ),
        _ => return Err(SigningError::Payload),
    };
    let primary = format!("HyperliquidTransaction:{primary}");
    let fields: Vec<Value> = fields
        .iter()
        .map(|(n, t)| json!({"name":n,"type":t}))
        .collect();
    let chain = action["signatureChainId"]
        .as_str()
        .and_then(|s| u64::from_str_radix(s.trim_start_matches("0x"), 16).ok())
        .ok_or(SigningError::Payload)?;
    serde_json::from_value(json!({"primaryType":primary,"types":{primary:fields},"domain":{"name":"HyperliquidSignTransaction","version":"1","chainId":chain,"verifyingContract":Address::zero()},"message":action})).map_err(|_|SigningError::Payload)
}
pub fn user_digest(action: &Value) -> Result<[u8; 32]> {
    user_typed_data(action)?
        .encode_eip712()
        .map_err(|_| SigningError::Payload)
}
pub fn wire_signature(sig: &Signature) -> Value {
    json!({"r":format!("0x{:064x}",sig.r),"s":format!("0x{:064x}",sig.s),"v":sig.v})
}
pub fn verify_signature(sig: &Signature, digest: [u8; 32], expected: Address) -> Result<()> {
    if sig
        .recover(H256(digest))
        .map_err(|_| SigningError::WrongSigner)?
        != expected
    {
        return Err(SigningError::WrongSigner);
    }
    Ok(())
}

/// The provider's wallet API.
const PRIVY_API: &str = "https://api.privy.io";
pub struct PrivySigner {
    wallet_id: String,
    address: Address,
    app_id: String,
    app_secret: SecretString,
    auth_key: p256::ecdsa::SigningKey,
    http: reqwest::Client,
    /// Where the wallet API is. Only tests point it anywhere else.
    api: String,
}
impl PrivySigner {
    pub fn new(
        wallet_id: String,
        address: Address,
        app_id: String,
        app_secret: SecretString,
        auth_key: SecretString,
    ) -> Result<Self> {
        use base64::Engine;
        use p256::pkcs8::DecodePrivateKey;
        if wallet_id.is_empty()
            || wallet_id.len() > 200
            || !wallet_id
                .bytes()
                .all(|c| c.is_ascii_alphanumeric() || c == b'-' || c == b'_')
            || app_id.is_empty()
            || app_secret.expose_secret().is_empty()
        {
            return Err(SigningError::Credential);
        }
        let bytes = base64::engine::general_purpose::STANDARD
            .decode(auth_key.expose_secret().trim())
            .map_err(|_| SigningError::Credential)?;
        let protected = secrecy::SecretBox::new(bytes.into_boxed_slice());
        let auth_key = p256::ecdsa::SigningKey::from_pkcs8_der(protected.expose_secret())
            .map_err(|_| SigningError::Credential)?;
        Ok(Self {
            wallet_id,
            address,
            app_id,
            app_secret,
            auth_key,
            // Signers are built per account and per request; the pool is not.
            http: shared_http_client().map_err(|_| SigningError::Unavailable)?,
            api: PRIVY_API.into(),
        })
    }
    #[cfg(test)]
    fn at(mut self, api: &str) -> Self {
        self.api = api.into();
        self
    }
}
/// The provider's own short name for a refusal, when its answer carries one:
/// a word like `invalid_data`. It is the only part of the answer that is
/// logged. The sentence beside it can name a wallet or echo the request.
fn refusal_code(body: &Value) -> &str {
    body["code"]
        .as_str()
        .filter(|code| {
            (1..=64).contains(&code.len())
                && code
                    .bytes()
                    .all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == b'_')
        })
        .unwrap_or("unreadable")
}
#[async_trait]
impl Signer for PrivySigner {
    fn address(&self) -> Address {
        self.address
    }
    /// Signs a ready-made digest with the member's wallet.
    ///
    /// An Ethereum wallet does this through the wallet's RPC endpoint, method
    /// `secp256k1_sign`: the digest is signed as it is, with nothing put in
    /// front of it. The provider's `raw_sign` endpoint, which this used
    /// before, refuses Ethereum wallets ("not supported for this low-level
    /// signature endpoint") and is only for chains that have no RPC methods.
    ///
    /// A refusal is logged with the provider's status and its short code, so
    /// that a change on its side reads as what it is and not as a signing
    /// failure without a cause.
    async fn sign_digest(&self, digest: [u8; 32]) -> Result<Signature> {
        use base64::Engine;
        use p256::ecdsa::signature::Signer as P256Signer;
        let url = format!("{}/v1/wallets/{}/rpc", self.api, self.wallet_id);
        let expiry = (chrono::Utc::now().timestamp_millis() + 60_000).to_string();
        let body = json!({"method":"secp256k1_sign","params":{"hash":format!("0x{}",hex::encode(digest))}});
        let headers = json!({"privy-app-id":self.app_id,"privy-request-expiry":expiry});
        let canonical = serde_json_canonicalizer::to_vec(
            &json!({"version":1,"method":"POST","url":url,"body":body,"headers":headers}),
        )
        .map_err(|_| SigningError::Payload)?;
        let signature: p256::ecdsa::Signature = self.auth_key.sign(&canonical);
        let response = self
            .http
            .post(url)
            .basic_auth(&self.app_id, Some(self.app_secret.expose_secret()))
            .header("privy-app-id", &self.app_id)
            .header("privy-request-expiry", expiry)
            .header(
                "privy-authorization-signature",
                base64::engine::general_purpose::STANDARD.encode(signature.to_der().as_bytes()),
            )
            .json(&body)
            .send()
            .await
            .map_err(|error| {
                tracing::warn!(
                    target: "astradia::signing",
                    error_code = "signing_service_unavailable",
                    timed_out = error.is_timeout(),
                    connect_failed = error.is_connect(),
                    "The wallet provider could not be reached to sign"
                );
                SigningError::Unavailable
            })?;
        let status = response.status();
        if response.content_length().is_some_and(|n| n > 65536) {
            return Err(SigningError::Rejected);
        }
        let answer = crate::http::bounded_json(response, 65536).await;
        if !status.is_success() {
            let answer = answer.unwrap_or(Value::Null);
            tracing::warn!(
                target: "astradia::signing",
                error_code = "signing_service_rejected",
                provider_status = status.as_u16(),
                provider_code = refusal_code(&answer),
                "The wallet provider refused to sign"
            );
            return Err(SigningError::Rejected);
        }
        let response = answer.map_err(|_| SigningError::Rejected)?;
        let raw = response["data"]["signature"]
            .as_str()
            .or(response["signature"].as_str())
            .ok_or(SigningError::Rejected)?;
        let mut sig = Signature::from_str(raw).map_err(|_| SigningError::Rejected)?;
        if sig.v < 27 {
            sig.v += 27;
        }
        verify_signature(&sig, digest, self.address).inspect_err(|_| {
            tracing::warn!(
                target: "astradia::signing",
                error_code = "signing_wallet_mismatch",
                "The wallet provider signed with a different wallet than the one on record"
            );
        })?;
        Ok(sig)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use axum::{
        Json, Router,
        extract::{Path, State},
        http::{HeaderMap, StatusCode},
        routing::post,
    };
    use base64::Engine;
    use p256::ecdsa::signature::Verifier;
    use std::sync::{Arc, Mutex};

    const APP: &str = "app-under-test";
    const WALLET: &str = "wallet_under_test";

    /// What the stand-in provider was asked, and how it is told to answer.
    #[derive(Default)]
    struct Provider {
        base: String,
        /// The key the member's wallet signs with at the provider.
        wallet_key: [u8; 32],
        /// The app's authorization key, as the provider knows it.
        authorized: Option<p256::ecdsa::VerifyingKey>,
        calls: Vec<String>,
    }
    type Shared = Arc<Mutex<Provider>>;

    /// The provider's RPC endpoint: checks the request the way the provider
    /// does and signs the digest with the wallet's key.
    async fn rpc(
        State(provider): State<Shared>,
        Path(wallet): Path<String>,
        headers: HeaderMap,
        Json(body): Json<Value>,
    ) -> (StatusCode, Json<Value>) {
        let mut provider = provider.lock().unwrap();
        provider.calls.push(format!("rpc:{}", body["method"]));
        let header = |name: &str| {
            headers
                .get(name)
                .and_then(|v| v.to_str().ok())
                .unwrap_or("")
        };
        let refuse = |code: &str| {
            (
                StatusCode::UNAUTHORIZED,
                Json(json!({"error":format!("refused for {wallet}"),"code":code})),
            )
        };
        if wallet != WALLET || !header("authorization").starts_with("Basic ") {
            return refuse("missing_or_invalid_token");
        }
        // The app's signature must cover exactly this request: this URL, this
        // body and the headers the provider counts.
        let url = format!("{}/v1/wallets/{wallet}/rpc", provider.base);
        let signed = serde_json_canonicalizer::to_vec(&json!({
            "version":1,"method":"POST","url":url,"body":body,
            "headers":{"privy-app-id":header("privy-app-id"),"privy-request-expiry":header("privy-request-expiry")},
        }))
        .unwrap();
        let authorization = base64::engine::general_purpose::STANDARD
            .decode(header("privy-authorization-signature"))
            .ok()
            .and_then(|der| p256::ecdsa::Signature::from_der(&der).ok());
        let authorized = match (&provider.authorized, authorization) {
            (Some(key), Some(signature)) => key.verify(&signed, &signature).is_ok(),
            _ => false,
        };
        let expires: i64 = header("privy-request-expiry").parse().unwrap_or(0);
        if header("privy-app-id") != APP
            || !authorized
            || expires <= chrono::Utc::now().timestamp_millis()
        {
            return refuse("invalid_authorization_signature");
        }
        let digest = body["params"]["hash"]
            .as_str()
            .and_then(|hash| hash.strip_prefix("0x"))
            .and_then(|hash| hex::decode(hash).ok())
            .filter(|digest| digest.len() == 32);
        let (Some(digest), true) = (digest, body["method"] == "secp256k1_sign") else {
            return (
                StatusCode::BAD_REQUEST,
                Json(json!({"error":"Invalid request","code":"invalid_data"})),
            );
        };
        let wallet = LocalWallet::from_bytes(&provider.wallet_key).unwrap();
        let signature = wallet.sign_hash(H256::from_slice(&digest)).unwrap();
        (
            StatusCode::OK,
            Json(
                json!({"method":"secp256k1_sign","data":{"signature":format!("0x{signature}"),"encoding":"hex"}}),
            ),
        )
    }

    /// The endpoint this signer used before, answering as the provider does now.
    async fn raw_sign(State(provider): State<Shared>) -> (StatusCode, Json<Value>) {
        provider.lock().unwrap().calls.push("raw_sign".into());
        (
            StatusCode::BAD_REQUEST,
            Json(
                json!({"error":"ethereum wallets are not supported for this low-level signature endpoint.","code":"invalid_data"}),
            ),
        )
    }

    /// A stand-in provider, and a signer for the member's wallet that talks to it.
    async fn provider(wallet_key: [u8; 32]) -> (Shared, PrivySigner) {
        use p256::pkcs8::EncodePrivateKey;
        let app_key = p256::ecdsa::SigningKey::from_slice(&[7; 32]).unwrap();
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let base = format!("http://{}", listener.local_addr().unwrap());
        let shared = Arc::new(Mutex::new(Provider {
            base: base.clone(),
            wallet_key,
            authorized: Some(*app_key.verifying_key()),
            calls: Vec::new(),
        }));
        let routes = Router::new()
            .route("/v1/wallets/{wallet}/rpc", post(rpc))
            .route("/v1/wallets/{wallet}/raw_sign", post(raw_sign))
            .with_state(shared.clone());
        tokio::spawn(async move { axum::serve(listener, routes).await.unwrap() });
        let der = app_key.to_pkcs8_der().unwrap();
        let encoded = base64::engine::general_purpose::STANDARD.encode(der.as_bytes());
        let signer = PrivySigner::new(
            WALLET.into(),
            LocalWallet::from_bytes(&[1; 32]).unwrap().address(),
            APP.into(),
            "s".repeat(40).into(),
            encoded.into(),
        )
        .unwrap()
        .at(&base);
        (shared, signer)
    }

    #[tokio::test]
    async fn a_provider_wallet_signs_a_digest_through_the_rpc_method_for_ethereum() {
        let (provider, signer) = provider([1; 32]).await;
        let digest = l1_digest(keccak256("an order"), false);
        let signature = signer.sign_digest(digest).await.unwrap();
        // The digest itself is signed, with nothing put in front of it: the
        // signature recovers to the member's wallet, as the exchange requires.
        assert_eq!(signature.recover(H256(digest)).unwrap(), signer.address());
        assert!(signature.v == 27 || signature.v == 28);
        // One request, to the RPC method. The endpoint that refuses Ethereum
        // wallets is not asked.
        assert_eq!(provider.lock().unwrap().calls, ["rpc:\"secp256k1_sign\""]);
    }

    #[tokio::test]
    async fn a_refusal_or_a_signature_by_another_wallet_never_becomes_a_signature() {
        // The provider no longer accepts the app's authorization key.
        let (provider, signer) = provider([1; 32]).await;
        provider.lock().unwrap().authorized = None;
        let digest = l1_digest(keccak256("an order"), false);
        assert!(matches!(
            signer.sign_digest(digest).await,
            Err(SigningError::Rejected)
        ));
        // The provider signs with a wallet that is not the member's on record.
        let (_, signer) = self::provider([2; 32]).await;
        assert!(matches!(
            signer.sign_digest(digest).await,
            Err(SigningError::WrongSigner)
        ));
    }

    #[test]
    fn only_the_providers_short_code_is_taken_from_a_refusal() {
        for (body, code) in [
            (
                json!({"error":"ethereum wallets are not supported","code":"invalid_data"}),
                "invalid_data",
            ),
            (
                json!({"code":"missing_or_invalid_privy_app_id"}),
                "missing_or_invalid_privy_app_id",
            ),
            // Anything that is not a short machine word could carry a wallet,
            // a hash or the request itself, and is not repeated.
            (
                json!({"code":"Wallet wallet_under_test not found"}),
                "unreadable",
            ),
            (json!({"code":"0xabc DEF"}), "unreadable"),
            (json!({"code":"x".repeat(65)}), "unreadable"),
            (json!({"code":""}), "unreadable"),
            (json!({"code":7}), "unreadable"),
            (json!({"error":"no code"}), "unreadable"),
            (Value::Null, "unreadable"),
        ] {
            assert_eq!(refusal_code(&body), code, "{body}");
        }
    }
}

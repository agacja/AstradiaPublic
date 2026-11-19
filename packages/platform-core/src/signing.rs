//! Hyperliquid SDK wire types with the existing Privy raw-digest signing path.
//! Keep the exact named MessagePack field order and canonical decimal strings.
use crate::http::http_client;
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

pub struct PrivySigner {
    wallet_id: String,
    address: Address,
    app_id: String,
    app_secret: SecretString,
    auth_key: p256::ecdsa::SigningKey,
    http: reqwest::Client,
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
            http: http_client().map_err(|_| SigningError::Unavailable)?,
        })
    }
}
#[async_trait]
impl Signer for PrivySigner {
    fn address(&self) -> Address {
        self.address
    }
    async fn sign_digest(&self, digest: [u8; 32]) -> Result<Signature> {
        use base64::Engine;
        use p256::ecdsa::signature::Signer as P256Signer;
        let url = format!(
            "https://api.privy.io/v1/wallets/{}/raw_sign",
            self.wallet_id
        );
        let expiry = (chrono::Utc::now().timestamp_millis() + 60_000).to_string();
        let body = json!({"params":{"hash":format!("0x{}",hex::encode(digest))}});
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
            .map_err(|_| SigningError::Unavailable)?;
        if !response.status().is_success() {
            return Err(SigningError::Rejected);
        }
        if response.content_length().is_some_and(|n| n > 65536) {
            return Err(SigningError::Rejected);
        }
        let response = crate::http::bounded_json(response, 65536)
            .await
            .map_err(|_| SigningError::Rejected)?;
        let raw = response["data"]["signature"]
            .as_str()
            .or(response["signature"].as_str())
            .ok_or(SigningError::Rejected)?;
        let mut sig = Signature::from_str(raw).map_err(|_| SigningError::Rejected)?;
        if sig.v < 27 {
            sig.v += 27;
        }
        verify_signature(&sig, digest, self.address)?;
        Ok(sig)
    }
}

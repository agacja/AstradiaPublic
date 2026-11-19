//! Reads existing AES-GCM ciphertext without rewriting any user row.
use aes_gcm::{
    Aes256Gcm, AesGcm, KeyInit,
    aead::{Aead, OsRng, consts::U16, rand_core::RngCore},
    aes::Aes256,
};
use secrecy::{ExposeSecret, SecretString};
#[derive(Debug, thiserror::Error)]
#[error("Encrypted credential unavailable")]
pub struct Error;
pub struct Vault {
    key: secrecy::SecretBox<[u8; 32]>,
}
impl Vault {
    pub fn new(key: SecretString) -> Result<Self, Error> {
        let decoded = hex::decode(key.expose_secret().trim()).map_err(|_| Error)?;
        let bytes: [u8; 32] = decoded.try_into().map_err(|_| Error)?;
        Ok(Self {
            key: secrecy::SecretBox::new(Box::new(bytes)),
        })
    }
    pub fn encrypt(&self, plaintext: &SecretString) -> Result<Vec<u8>, Error> {
        let mut nonce = [0u8; 12];
        OsRng.fill_bytes(&mut nonce);
        let cipher = Aes256Gcm::new_from_slice(self.key.expose_secret()).map_err(|_| Error)?;
        let mut result = vec![1];
        result.extend_from_slice(&nonce);
        result.extend(
            cipher
                .encrypt((&nonce).into(), plaintext.expose_secret().as_bytes())
                .map_err(|_| Error)?,
        );
        Ok(result)
    }
    pub fn decrypt(&self, blob: &[u8]) -> Result<SecretString, Error> {
        if blob.len() < 29 || blob[0] != 1 {
            return Err(Error);
        }
        let cipher = Aes256Gcm::new_from_slice(self.key.expose_secret()).map_err(|_| Error)?;
        let plaintext = cipher
            .decrypt(blob[1..13].into(), &blob[13..])
            .map_err(|_| Error)?;
        Ok(String::from_utf8(plaintext).map_err(|_| Error)?.into())
    }
    pub fn legacy(&self, blob: &str) -> Result<SecretString, Error> {
        let parts: Vec<_> = blob.split(':').collect();
        if parts.len() != 3 {
            return Err(Error);
        }
        let nonce = hex::decode(parts[0]).map_err(|_| Error)?;
        let tag = hex::decode(parts[1]).map_err(|_| Error)?;
        let mut ct = hex::decode(parts[2]).map_err(|_| Error)?;
        if tag.len() != 16 {
            return Err(Error);
        }
        ct.extend(tag);
        let plain = match nonce.len() {
            12 => Aes256Gcm::new_from_slice(self.key.expose_secret())
                .map_err(|_| Error)?
                .decrypt(nonce.as_slice().into(), ct.as_slice()),
            16 => AesGcm::<Aes256, U16>::new_from_slice(self.key.expose_secret())
                .map_err(|_| Error)?
                .decrypt(nonce.as_slice().into(), ct.as_slice()),
            _ => return Err(Error),
        }
        .map_err(|_| Error)?;
        Ok(String::from_utf8(plain).map_err(|_| Error)?.into())
    }
}
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn v1_roundtrip_and_tampering() {
        let v = Vault::new("11".repeat(32).into()).unwrap();
        let s: SecretString = "dummy-trading-key".into();
        let mut b = v.encrypt(&s).unwrap();
        assert_eq!(v.decrypt(&b).unwrap().expose_secret(), s.expose_secret());
        b[14] ^= 1;
        assert!(v.decrypt(&b).is_err());
        assert!(v.decrypt(&[0; 29]).is_err());
    }
}
#[cfg(test)]
mod compatibility {
    use super::*;
    #[test]
    fn node_legacy_and_bytea_blobs_decrypt_without_migration() {
        let cases: serde_json::Value =
            serde_json::from_str(include_str!("../tests/fixtures/vault.json")).unwrap();
        let vault = Vault::new(cases["key"].as_str().unwrap().into()).unwrap();
        for c in cases["fixtures"].as_array().unwrap() {
            assert_eq!(
                vault
                    .legacy(c["legacy"].as_str().unwrap())
                    .unwrap()
                    .expose_secret(),
                cases["plaintext"].as_str().unwrap()
            );
            if let Some(blob) = c["bytea"].as_str() {
                assert_eq!(
                    vault
                        .decrypt(&hex::decode(blob).unwrap())
                        .unwrap()
                        .expose_secret(),
                    cases["plaintext"].as_str().unwrap()
                );
            }
        }
    }
}

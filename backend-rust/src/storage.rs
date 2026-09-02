use std::sync::Arc;
use std::time::Duration;

use object_store::aws::{AmazonS3, AmazonS3Builder};
use object_store::path::Path as ObjPath;
use object_store::signer::Signer;
use object_store::ObjectStore;
use reqwest::Method;

/// MinIO-compatible object storage: an internal client for uploads and a
/// public-endpoint client for presigning URLs handed to the browser (mirrors
/// the Python `MediaStorage` internal/public client split).
#[derive(Clone)]
pub struct S3Storage {
    store: Arc<AmazonS3>,
    signer: Arc<AmazonS3>,
}

impl S3Storage {
    pub fn new(
        bucket: &str,
        endpoint: &str,
        public_endpoint: &str,
        access_key: &str,
        secret_key: &str,
        region: &str,
    ) -> anyhow::Result<Self> {
        let build = |ep: &str| -> anyhow::Result<AmazonS3> {
            Ok(AmazonS3Builder::new()
                .with_bucket_name(bucket)
                .with_endpoint(ep)
                .with_region(region)
                .with_access_key_id(access_key)
                .with_secret_access_key(secret_key)
                .with_allow_http(ep.starts_with("http://"))
                .with_virtual_hosted_style_request(false)
                .build()?)
        };
        Ok(Self {
            store: Arc::new(build(endpoint)?),
            signer: Arc::new(build(public_endpoint)?),
        })
    }

    pub async fn put_file(
        &self,
        key: &str,
        path: &std::path::Path,
        content_type: &str,
    ) -> anyhow::Result<()> {
        let bytes = tokio::fs::read(path).await?;
        self.put_bytes(key, bytes, content_type).await
    }

    pub async fn put_bytes(&self, key: &str, bytes: Vec<u8>, content_type: &str) -> anyhow::Result<()> {
        let opts = object_store::PutOptions {
            attributes: object_store::Attributes::from_iter([(
                object_store::Attribute::ContentType,
                content_type.to_string(),
            )]),
            ..Default::default()
        };
        self.store.put_opts(&ObjPath::from(key), bytes.into(), opts).await?;
        Ok(())
    }

    pub async fn presign_get(&self, key: &str, expires: Duration) -> anyhow::Result<String> {
        let url = self
            .signer
            .signed_url(Method::GET, &ObjPath::from(key), expires)
            .await?;
        Ok(url.to_string())
    }
}

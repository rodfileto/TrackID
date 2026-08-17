"""
S3-compatible object storage for persisted video files and face crops.

Points at MinIO in dev/docker (see docker-compose*.yml); swapping to real
AWS S3 later is a config change only (S3_ENDPOINT_URL/credentials/SSL),
not a code change.
"""
import boto3
from botocore.exceptions import ClientError

from app.core.config import settings


class MediaStorage:
    def __init__(self):
        self._client = boto3.client(
            "s3",
            endpoint_url=settings.S3_ENDPOINT_URL,
            aws_access_key_id=settings.S3_ACCESS_KEY_ID,
            aws_secret_access_key=settings.S3_SECRET_ACCESS_KEY,
            region_name=settings.S3_REGION_NAME,
            use_ssl=settings.S3_USE_SSL,
        )
        # Presigned URLs are signed with (and embed) whatever endpoint the
        # signing client is configured with. A browser can't resolve the
        # container-network hostname used for server-side uploads, so URLs
        # handed to the frontend are signed with a separate client pointed
        # at the host-reachable S3_PUBLIC_URL instead.
        self._public_client = (
            self._client
            if settings.S3_PUBLIC_URL == settings.S3_ENDPOINT_URL
            else boto3.client(
                "s3",
                endpoint_url=settings.S3_PUBLIC_URL,
                aws_access_key_id=settings.S3_ACCESS_KEY_ID,
                aws_secret_access_key=settings.S3_SECRET_ACCESS_KEY,
                region_name=settings.S3_REGION_NAME,
            )
        )
        self._bucket = settings.S3_BUCKET_NAME

    def ensure_bucket(self) -> None:
        try:
            self._client.head_bucket(Bucket=self._bucket)
        except ClientError as exc:
            status = exc.response.get("Error", {}).get("Code")
            if status in ("404", "NoSuchBucket"):
                self._client.create_bucket(Bucket=self._bucket)
            else:
                raise

    def upload_file(self, local_path: str, key: str, content_type: str | None = None) -> str:
        extra_args = {"ContentType": content_type} if content_type else None
        self._client.upload_file(local_path, self._bucket, key, ExtraArgs=extra_args)
        return key

    def upload_bytes(self, data: bytes, key: str, content_type: str | None = None) -> str:
        kwargs = {"ContentType": content_type} if content_type else {}
        self._client.put_object(Bucket=self._bucket, Key=key, Body=data, **kwargs)
        return key

    def generate_presigned_url(self, key: str, expires_in: int = 3600) -> str:
        return self._public_client.generate_presigned_url(
            "get_object", Params={"Bucket": self._bucket, "Key": key}, ExpiresIn=expires_in
        )


_storage = MediaStorage()


def get_media_storage() -> MediaStorage:
    return _storage

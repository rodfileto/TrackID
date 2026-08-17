"""
ORM models for persisted video processing results.

Mirrors the shape VideoProcessingService.process_video_for_best_faces()
already returns: a Video (one processing run) contains PersonTracks (one per
DBSCAN cluster, i.e. one per person seen in that video), each of which
contains FaceDetections (one per detected face instance across frames).

This is the tactical/operational layer only - fast, mutable, no legal
weight. It is not the forensic-evidentiary layer described in
docs/PAPER3_FORENSIC_EVIDENTIARY.md.
"""
import uuid

from pgvector.sqlalchemy import Vector
from sqlalchemy import (
    ARRAY,
    Boolean,
    CheckConstraint,
    Column,
    DateTime,
    Float,
    ForeignKey,
    Index,
    Integer,
    String,
    UniqueConstraint,
    func,
)
from sqlalchemy.dialects.postgresql import UUID
from sqlalchemy.orm import relationship

from app.core.database import Base

EMBEDDING_DIM = 512


class Video(Base):
    __tablename__ = "videos"

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    job_id = Column(String, index=True, nullable=True)
    original_filename = Column(String, nullable=True)
    content_type = Column(String, nullable=True)
    video_storage_key = Column(String, nullable=True)

    # Processing params used for this run (kept queryable for evaluation
    # work - see docs/PAPER1_TACTICAL_LINKING.md on recovering the
    # "threshold set used").
    interval_seconds = Column(Float, nullable=False)
    embed_interval_seconds = Column(Float, nullable=False)
    min_quality = Column(Float, nullable=False)
    min_blur = Column(Float, nullable=False)
    full_detection_every_frame = Column(Boolean, nullable=False, default=False)
    iou_threshold = Column(Float, nullable=False)
    cluster_eps = Column(Float, nullable=False)
    cluster_min_samples = Column(Integer, nullable=False)

    expected_frames = Column(Integer, nullable=False, default=0)
    frame_count = Column(Integer, nullable=False, default=0)
    total_detections = Column(Integer, nullable=False, default=0)
    track_count = Column(Integer, nullable=False, default=0)

    created_at = Column(DateTime(timezone=True), server_default=func.now())

    person_tracks = relationship(
        "PersonTrack", back_populates="video", cascade="all, delete-orphan"
    )
    face_detections = relationship(
        "FaceDetection", back_populates="video", cascade="all, delete-orphan"
    )


class PersonTrack(Base):
    __tablename__ = "person_tracks"
    __table_args__ = (
        UniqueConstraint("video_id", "track_local_id", name="uq_person_track_video_local_id"),
    )

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    video_id = Column(
        UUID(as_uuid=True), ForeignKey("videos.id", ondelete="CASCADE"), nullable=False, index=True
    )
    track_local_id = Column(Integer, nullable=False)
    # use_alter breaks the circular FK cycle with face_detections.person_track_id:
    # this constraint is emitted via ALTER TABLE after both tables exist,
    # instead of inline in CREATE TABLE.
    best_face_detection_id = Column(
        UUID(as_uuid=True),
        ForeignKey(
            "face_detections.id",
            ondelete="SET NULL",
            use_alter=True,
            name="fk_person_tracks_best_face_detection_id",
        ),
        nullable=True,
    )
    face_count = Column(Integer, nullable=False, default=0)
    representative_embedding = Column(Vector(EMBEDDING_DIM), nullable=True)

    created_at = Column(DateTime(timezone=True), server_default=func.now())

    video = relationship("Video", back_populates="person_tracks")
    detections = relationship(
        "FaceDetection",
        back_populates="person_track",
        foreign_keys="FaceDetection.person_track_id",
    )
    # post_update breaks the mapper-level cycle this relationship forms
    # with FaceDetection.person_track (each depends on the other's table):
    # without it, the unit-of-work's insert ordering for a combined
    # PersonTrack+FaceDetection flush is unreliable and can violate
    # face_detections_person_track_id_fkey. With it, SQLAlchemy inserts
    # both sides first and resolves this one FK via a follow-up UPDATE.
    best_face = relationship(
        "FaceDetection", foreign_keys=[best_face_detection_id], post_update=True
    )


class FaceDetection(Base):
    __tablename__ = "face_detections"
    __table_args__ = (
        CheckConstraint("bbox IS NULL OR array_length(bbox, 1) = 4", name="ck_face_detection_bbox_len"),
    )

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    video_id = Column(
        UUID(as_uuid=True), ForeignKey("videos.id", ondelete="CASCADE"), nullable=False, index=True
    )
    person_track_id = Column(
        UUID(as_uuid=True),
        ForeignKey("person_tracks.id", ondelete="CASCADE"),
        nullable=True,
        index=True,
    )

    frame_number = Column(Integer, nullable=False)
    timestamp_seconds = Column(Float, nullable=False)
    confidence = Column(Float, nullable=False)
    quality_score = Column(Float, nullable=True)
    blur_score = Column(Float, nullable=True)
    bbox = Column(ARRAY(Float), nullable=True)
    is_embedding = Column(Boolean, nullable=False)
    embedding = Column(Vector(EMBEDDING_DIM), nullable=True)
    estimated_age = Column(Integer, nullable=True)
    estimated_gender = Column(String, nullable=True)
    crop_storage_key = Column(String, nullable=True)

    created_at = Column(DateTime(timezone=True), server_default=func.now())

    video = relationship("Video", back_populates="face_detections")
    person_track = relationship(
        "PersonTrack", back_populates="detections", foreign_keys=[person_track_id]
    )


Index(
    "ix_face_detections_embedding_hnsw",
    FaceDetection.embedding,
    postgresql_using="hnsw",
    postgresql_ops={"embedding": "vector_cosine_ops"},
)
Index(
    "ix_person_tracks_representative_embedding_hnsw",
    PersonTrack.representative_embedding,
    postgresql_using="hnsw",
    postgresql_ops={"representative_embedding": "vector_cosine_ops"},
)

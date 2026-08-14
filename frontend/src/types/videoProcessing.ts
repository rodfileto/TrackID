export interface VideoFaceDetection {
  frame_number: number;
  timestamp_seconds: number;
  confidence: number;
  quality_score: number | null;
  blur_score: number | null;
  bbox: number[] | null;
  is_embedding: boolean;
  estimated_age: number | null;
  estimated_gender: string | null;
  cluster_id: number | null;
  face_crop: string | null;
}

export interface VideoTrack {
  track_id: number;
  best_face: VideoFaceDetection;
  all_faces: VideoFaceDetection[];
}

export interface VideoProcessingResponse {
  tracks: VideoTrack[];
  track_count: number;
}

export interface VideoProcessingParams {
  interval_seconds: number;
  embed_interval_seconds: number;
  min_quality: number;
  min_blur: number;
  full_detection_every_frame: boolean;
  iou_threshold: number;
  cluster_eps: number;
  cluster_min_samples: number;
  include_crops: boolean;
}

export const DEFAULT_VIDEO_PROCESSING_PARAMS: VideoProcessingParams = {
  interval_seconds: 0.5,
  embed_interval_seconds: 1.0,
  min_quality: 0,
  min_blur: 50,
  full_detection_every_frame: false,
  iou_threshold: 0.15,
  cluster_eps: 0.45,
  cluster_min_samples: 2,
  include_crops: true,
};

export async function processVideo(
  file: File,
  params: VideoProcessingParams,
): Promise<VideoProcessingResponse> {
  const query = new URLSearchParams({
    interval_seconds: String(params.interval_seconds),
    embed_interval_seconds: String(params.embed_interval_seconds),
    min_quality: String(params.min_quality),
    min_blur: String(params.min_blur),
    full_detection_every_frame: String(params.full_detection_every_frame),
    iou_threshold: String(params.iou_threshold),
    cluster_eps: String(params.cluster_eps),
    cluster_min_samples: String(params.cluster_min_samples),
    include_crops: String(params.include_crops),
  });

  const formData = new FormData();
  formData.append("file", file);

  const response = await fetch(`/api/v1/process-video?${query.toString()}`, {
    method: "POST",
    body: formData,
  });

  if (!response.ok) {
    const detail = await response.json().catch(() => null);
    throw new Error(detail?.detail || `Request failed with status ${response.status}`);
  }

  return response.json();
}

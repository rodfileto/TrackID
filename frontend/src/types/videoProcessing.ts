import { authHeaders } from "../auth";

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
  video_id: string | null;
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

export interface VideoSummary {
  video_id: string;
  original_filename: string | null;
  created_at: string;
  track_count: number;
  frame_count: number;
  total_detections: number;
}

export interface VideoListResponse {
  items: VideoSummary[];
  total: number;
  page: number;
  page_size: number;
}

export interface VideoJobStatus {
  job_id: string;
  status: "processing" | "completed" | "failed";
  frame_count: number;
  expected_frames: number;
  percent: number;
  elapsed_seconds: number;
  eta_seconds: number | null;
  error: string | null;
  result: VideoProcessingResponse | null;
}

async function parseErrorDetail(response: Response): Promise<string> {
  const detail = await response.json().catch(() => null);
  return detail?.detail || `Request failed with status ${response.status}`;
}

export async function startVideoProcessing(
  file: File,
  params: VideoProcessingParams,
): Promise<string> {
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
    headers: authHeaders(),
    body: formData,
  });

  if (!response.ok) {
    throw new Error(await parseErrorDetail(response));
  }

  const data: { job_id: string } = await response.json();
  return data.job_id;
}

export async function fetchVideoJobStatus(jobId: string): Promise<VideoJobStatus> {
  const response = await fetch(`/api/v1/process-video/${jobId}`, {
    headers: authHeaders(),
  });
  if (!response.ok) {
    throw new Error(await parseErrorDetail(response));
  }
  return response.json();
}

/**
 * Lists previously processed videos (persisted to the database), newest
 * first, one page at a time.
 */
export async function fetchVideoList(
  page: number,
  pageSize: number,
): Promise<VideoListResponse> {
  const query = new URLSearchParams({
    page: String(page),
    page_size: String(pageSize),
  });
  const response = await fetch(`/api/v1/videos?${query.toString()}`, {
    headers: authHeaders(),
  });
  if (!response.ok) {
    throw new Error(await parseErrorDetail(response));
  }
  return response.json();
}

/**
 * Fetches the persisted version of a completed video's results (from
 * Postgres + object storage), which - unlike the in-memory job result -
 * has a real crop image for every detection, not just each track's best
 * face.
 */
export async function fetchPersistedVideo(videoId: string): Promise<VideoProcessingResponse> {
  const response = await fetch(`/api/v1/videos/${videoId}`, {
    headers: authHeaders(),
  });
  if (!response.ok) {
    throw new Error(await parseErrorDetail(response));
  }
  return response.json();
}

/**
 * Starts video processing, then polls until it completes or fails,
 * reporting progress along the way. Once complete, prefers the persisted
 * (DB + object storage backed) version of the results over the ephemeral
 * in-memory one - falling back to the in-memory result if persistence
 * didn't happen (e.g. it failed server-side, or is still catching up).
 */
export async function processVideo(
  file: File,
  params: VideoProcessingParams,
  onProgress?: (status: VideoJobStatus) => void,
  pollIntervalMs = 700,
): Promise<VideoProcessingResponse> {
  const jobId = await startVideoProcessing(file, params);

  while (true) {
    const status = await fetchVideoJobStatus(jobId);
    onProgress?.(status);

    if (status.status === "completed" && status.result) {
      if (status.result.video_id) {
        try {
          return await fetchPersistedVideo(status.result.video_id);
        } catch {
          // Persistence may have failed server-side despite the job
          // completing - fall back to the ephemeral result rather than
          // failing the whole request.
          return status.result;
        }
      }
      return status.result;
    }
    if (status.status === "failed") {
      throw new Error(status.error || "Video processing failed");
    }

    await new Promise((resolve) => setTimeout(resolve, pollIntervalMs));
  }
}

use std::collections::HashMap;
use std::sync::Mutex;
use std::time::Instant;

use crate::types::Track;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum JobStatus {
    Processing,
    Completed,
    Failed,
}

impl JobStatus {
    pub fn as_str(self) -> &'static str {
        match self {
            JobStatus::Processing => "processing",
            JobStatus::Completed => "completed",
            JobStatus::Failed => "failed",
        }
    }
}

#[derive(Debug, Clone)]
pub struct JobSnapshot {
    pub job_id: String,
    pub status: JobStatus,
    pub frame_count: i32,
    pub expected_frames: i32,
    pub started_at: Instant,
    pub finished_at: Option<Instant>,
    pub tracks: Option<Vec<Track>>,
    pub video_id: Option<String>,
    pub error: Option<String>,
}

struct Job {
    job_id: String,
    status: JobStatus,
    frame_count: i32,
    expected_frames: i32,
    total_detections: i32,
    started_at: Instant,
    finished_at: Option<Instant>,
    tracks: Option<Vec<Track>>,
    video_id: Option<String>,
    error: Option<String>,
}

impl Job {
    fn snapshot(&self) -> JobSnapshot {
        JobSnapshot {
            job_id: self.job_id.clone(),
            status: self.status,
            frame_count: self.frame_count,
            expected_frames: self.expected_frames,
            started_at: self.started_at,
            finished_at: self.finished_at,
            tracks: self.tracks.clone(),
            video_id: self.video_id.clone(),
            error: self.error.clone(),
        }
    }
}

/// In-memory job tracking, mirroring the Python `app/core/video_jobs.py`.
pub struct JobStore {
    jobs: Mutex<HashMap<String, Job>>,
}

impl JobStore {
    pub fn new() -> Self {
        Self {
            jobs: Mutex::new(HashMap::new()),
        }
    }

    pub fn create(&self, job_id: String) {
        let mut jobs = self.jobs.lock().unwrap();
        jobs.insert(
            job_id.clone(),
            Job {
                job_id,
                status: JobStatus::Processing,
                frame_count: 0,
                expected_frames: 0,
                total_detections: 0,
                started_at: Instant::now(),
                finished_at: None,
                tracks: None,
                video_id: None,
                error: None,
            },
        );
    }

    pub fn update_progress(&self, job_id: &str, frame_count: i32, expected_frames: i32, total_detections: i32) {
        let mut jobs = self.jobs.lock().unwrap();
        if let Some(job) = jobs.get_mut(job_id) {
            job.frame_count = frame_count;
            job.expected_frames = job.expected_frames.max(expected_frames);
            job.total_detections = total_detections;
        }
    }

    pub fn complete(&self, job_id: &str, tracks: Vec<Track>, video_id: Option<String>) {
        let mut jobs = self.jobs.lock().unwrap();
        if let Some(job) = jobs.get_mut(job_id) {
            job.status = JobStatus::Completed;
            job.tracks = Some(tracks);
            job.video_id = video_id;
            job.finished_at = Some(Instant::now());
        }
    }

    pub fn fail(&self, job_id: &str, error: String) {
        let mut jobs = self.jobs.lock().unwrap();
        if let Some(job) = jobs.get_mut(job_id) {
            job.status = JobStatus::Failed;
            job.error = Some(error);
            job.finished_at = Some(Instant::now());
        }
    }

    pub fn get(&self, job_id: &str) -> Option<JobSnapshot> {
        let jobs = self.jobs.lock().unwrap();
        jobs.get(job_id).map(Job::snapshot)
    }
}

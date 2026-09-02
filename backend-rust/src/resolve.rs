use sqlx::PgPool;
use uuid::Uuid;

use crate::db;
use crate::graph::GraphService;
use crate::types::ObservationOutcome;

#[derive(Clone)]
pub struct ResolutionConfig {
    pub tau_high: f64,
    pub tau_low: f64,
    pub pool_size: i64,
    pub top_k: usize,
}

/// Reuse-or-create the Person for an Identity. If the Identity already
/// IDENTIFIES a Person, reuse it; otherwise create a new Person named after
/// the identity and link both the IDENTIFIES edge and the identity's
/// enrollment FaceRecords.
pub async fn resolve_or_create_person_for_identity(
    graph: &GraphService,
    pool: &PgPool,
    identity_id: &str,
) -> anyhow::Result<String> {
    if let Some(pid) = graph.get_person_for_identity(identity_id).await? {
        return Ok(pid);
    }

    let identity = graph
        .get_identity(identity_id)
        .await?
        .ok_or_else(|| anyhow::anyhow!("identity {identity_id} not found"))?;

    let person_id = graph.create_person(&identity.full_name).await?;
    graph.attach_identity_to_person(identity_id, &person_id).await?;

    let fr_ids = db::enrollment_face_record_ids(pool, identity_id).await?;
    graph.link_face_records_to_person(&fr_ids, &person_id).await?;

    Ok(person_id)
}

/// The confidence-gated resolution control loop: match a new observation's
/// embedding against everyone already known, then auto-commit / queue /
/// create a new unidentified Person (tripartite policy — load-bearing for
/// Paper 1).
pub async fn resolve_observation(
    graph: &GraphService,
    pool: &PgPool,
    cfg: &ResolutionConfig,
    face_record_id: &str,
    embedding_id: Uuid,
    embedding: &[f64],
) -> anyhow::Result<ObservationOutcome> {
    let candidates = db::find_candidates(pool, embedding, cfg.pool_size, cfg.top_k).await?;
    let top = candidates.first().cloned();

    if let Some(top) = &top {
        if top.similarity >= cfg.tau_high {
            let person_id = if top.kind == "identity" {
                let iid = top.identity_id.clone().unwrap();
                resolve_or_create_person_for_identity(graph, pool, &iid).await?
            } else {
                top.person_id.clone().unwrap()
            };

            graph
                .link_face_record_to_person(face_record_id, &person_id)
                .await?;
            db::set_embedding_person_id(pool, embedding_id, &person_id).await?;
            graph
                .log_action(
                    "auto_match",
                    "system",
                    &[face_record_id.to_string(), person_id.clone()],
                    Some(top.similarity),
                    None,
                )
                .await?;

            return Ok(ObservationOutcome {
                outcome: "auto_committed",
                person_id: Some(person_id),
                review_item_id: None,
                candidates,
            });
        }

        if top.similarity >= cfg.tau_low {
            let item_id =
                db::create_review_item(pool, embedding_id, face_record_id, top, &candidates).await?;
            graph
                .log_action(
                    "queued_for_review",
                    "system",
                    &[face_record_id.to_string()],
                    Some(top.similarity),
                    None,
                )
                .await?;

            return Ok(ObservationOutcome {
                outcome: "queued",
                person_id: None,
                review_item_id: Some(item_id.to_string()),
                candidates,
            });
        }
    }

    // No candidate strong enough (or none at all): a brand-new, unidentified
    // Person — "unidentified" is a state, not a class.
    let person_id = graph
        .create_person(&format!("Unidentified Person ({})", &face_record_id[..8]))
        .await?;
    graph
        .link_face_record_to_person(face_record_id, &person_id)
        .await?;
    db::set_embedding_person_id(pool, embedding_id, &person_id).await?;
    graph
        .log_action(
            "new_person",
            "system",
            &[face_record_id.to_string(), person_id.clone()],
            top.as_ref().map(|t| t.similarity),
            None,
        )
        .await?;

    Ok(ObservationOutcome {
        outcome: "new_person",
        person_id: Some(person_id),
        review_item_id: None,
        candidates,
    })
}

/// Analyst accepts a specific person candidate.
pub async fn confirm_review_as_person(
    graph: &GraphService,
    pool: &PgPool,
    item_id: Uuid,
    person_id: &str,
    actor: &str,
) -> anyhow::Result<Option<()>> {
    let Some(item) = db::get_review_item(pool, item_id).await? else {
        return Ok(None);
    };
    if item.status != "pending" {
        return Ok(None);
    }

    graph
        .link_face_record_to_person(&item.face_record_id, person_id)
        .await?;
    db::set_embedding_person_id(pool, item.embedding_id, person_id).await?;
    db::set_review_status(pool, item_id, "confirmed", actor).await?;
    graph
        .log_action(
            "analyst_confirm",
            actor,
            &[item.face_record_id.clone(), person_id.to_string()],
            Some(item.similarity),
            None,
        )
        .await?;
    Ok(Some(()))
}

/// None of the candidates match — resolve to a newly-created Person.
pub async fn confirm_review_as_new(
    graph: &GraphService,
    pool: &PgPool,
    item_id: Uuid,
    name: &str,
    actor: &str,
) -> anyhow::Result<Option<()>> {
    let Some(item) = db::get_review_item(pool, item_id).await? else {
        return Ok(None);
    };
    if item.status != "pending" {
        return Ok(None);
    }

    let person_id = graph.create_person(name).await?;
    graph
        .link_face_record_to_person(&item.face_record_id, &person_id)
        .await?;
    db::set_embedding_person_id(pool, item.embedding_id, &person_id).await?;
    db::set_review_status(pool, item_id, "confirmed_new", actor).await?;
    graph
        .log_action(
            "analyst_confirm_new",
            actor,
            &[item.face_record_id.clone(), person_id.clone()],
            Some(item.similarity),
            None,
        )
        .await?;
    Ok(Some(()))
}

/// Analyst confirms this observation matches a specific pre-enrolled Identity.
pub async fn confirm_review_as_identity(
    graph: &GraphService,
    pool: &PgPool,
    item_id: Uuid,
    identity_id: &str,
    actor: &str,
) -> anyhow::Result<Option<()>> {
    let Some(item) = db::get_review_item(pool, item_id).await? else {
        return Ok(None);
    };
    if item.status != "pending" {
        return Ok(None);
    }

    let person_id = resolve_or_create_person_for_identity(graph, pool, identity_id).await?;
    graph
        .link_face_record_to_person(&item.face_record_id, &person_id)
        .await?;
    db::set_embedding_person_id(pool, item.embedding_id, &person_id).await?;
    db::set_review_status(pool, item_id, "confirmed_identity", actor).await?;
    graph
        .log_action(
            "analyst_confirm_identity",
            actor,
            &[
                item.face_record_id.clone(),
                person_id,
                identity_id.to_string(),
            ],
            Some(item.similarity),
            None,
        )
        .await?;
    Ok(Some(()))
}

/// No graph edge created; the embedding stays searchable for future matches.
pub async fn reject_review(
    graph: &GraphService,
    pool: &PgPool,
    item_id: Uuid,
    actor: &str,
    notes: Option<&str>,
) -> anyhow::Result<Option<()>> {
    let Some(item) = db::get_review_item(pool, item_id).await? else {
        return Ok(None);
    };
    if item.status != "pending" {
        return Ok(None);
    }

    db::set_review_status(pool, item_id, "rejected", actor).await?;
    graph
        .log_action(
            "analyst_reject",
            actor,
            &[item.face_record_id.clone()],
            None,
            notes,
        )
        .await?;
    Ok(Some(()))
}

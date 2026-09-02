use neo4rs::{query, Graph};
use uuid::Uuid;

/// Thin wrapper over the neo4rs Bolt connection for the face-record instance
/// layer: Person, FaceRecord, Identity, and the `face_record_of`/`IDENTIFIES`
/// edges, plus analyst-action audit nodes. All ids are client-generated UUIDs.
#[derive(Clone)]
pub struct GraphService {
    graph: Graph,
}

pub struct IdentityNode {
    pub id: String,
    pub full_name: String,
    pub document_type: Option<String>,
    pub document_number: Option<String>,
    pub created_at: String,
}

pub struct PersonNode {
    pub id: String,
    pub name: String,
    pub created_at: String,
}

impl GraphService {
    pub fn new(uri: &str) -> anyhow::Result<Self> {
        let graph = Graph::new(uri, "", "")?;
        Ok(Self { graph })
    }

    async fn run(&self, q: neo4rs::Query) -> anyhow::Result<()> {
        self.graph.run(q).await?;
        Ok(())
    }

    /// Run a query that returns no rows (CREATE/MERGE/SET). Exposed for the
    /// ontology seed, which issues many idempotent MERGEs.
    pub async fn run_query(&self, q: neo4rs::Query) -> anyhow::Result<()> {
        self.run(q).await
    }

    pub async fn create_person(&self, name: &str) -> anyhow::Result<String> {
        let id = Uuid::new_v4().to_string();
        self.run(
            query("CREATE (p:Person {id: $id, name: $name, created_at: $created_at})")
                .param("id", id.clone())
                .param("name", name.to_string())
                .param("created_at", chrono::Utc::now().to_rfc3339()),
        )
        .await?;
        Ok(id)
    }

    pub async fn create_face_record(&self, source: &str) -> anyhow::Result<String> {
        let id = Uuid::new_v4().to_string();
        self.run(
            query("CREATE (fr:FaceRecord {id: $id, source: $source, created_at: $created_at})")
                .param("id", id.clone())
                .param("source", source.to_string())
                .param("created_at", chrono::Utc::now().to_rfc3339()),
        )
        .await?;
        Ok(id)
    }

    pub async fn link_face_record_to_person(
        &self,
        face_record_id: &str,
        person_id: &str,
    ) -> anyhow::Result<()> {
        self.run(
            query(
                "MATCH (fr:FaceRecord {id: $fr}) MATCH (p:Person {id: $p})
                 CREATE (fr)-[:face_record_of]->(p)",
            )
            .param("fr", face_record_id.to_string())
            .param("p", person_id.to_string()),
        )
        .await?;
        Ok(())
    }

    pub async fn create_identity(
        &self,
        full_name: &str,
        document_type: Option<&str>,
        document_number: Option<&str>,
    ) -> anyhow::Result<String> {
        let id = Uuid::new_v4().to_string();
        self.run(
            query(
                "CREATE (i:Identity {id: $id, full_name: $full_name,
                    document_type: $doc_type, document_number: $doc_number,
                    created_at: $created_at})",
            )
            .param("id", id.clone())
            .param("full_name", full_name.to_string())
            .param("doc_type", document_type.unwrap_or("").to_string())
            .param("doc_number", document_number.unwrap_or("").to_string())
            .param("created_at", chrono::Utc::now().to_rfc3339()),
        )
        .await?;
        Ok(id)
    }

    pub async fn get_identity(&self, id: &str) -> anyhow::Result<Option<IdentityNode>> {
        let mut result = self
            .graph
            .execute(query("MATCH (i:Identity {id: $id}) RETURN i").param("id", id.to_string()))
            .await?;

        if let Some(row) = result.next().await? {
            let node: neo4rs::Node = row.get("i")?;
            let document_type: String = node.get("document_type")?;
            let document_number: String = node.get("document_number")?;
            Ok(Some(IdentityNode {
                id: id.to_string(),
                full_name: node.get("full_name")?,
                document_type: if document_type.is_empty() {
                    None
                } else {
                    Some(document_type)
                },
                document_number: if document_number.is_empty() {
                    None
                } else {
                    Some(document_number)
                },
                created_at: node.get("created_at")?,
            }))
        } else {
            Ok(None)
        }
    }

    pub async fn get_person(&self, id: &str) -> anyhow::Result<Option<PersonNode>> {
        let mut result = self
            .graph
            .execute(query("MATCH (p:Person {id: $id}) RETURN p").param("id", id.to_string()))
            .await?;

        if let Some(row) = result.next().await? {
            let node: neo4rs::Node = row.get("p")?;
            Ok(Some(PersonNode {
                id: id.to_string(),
                name: node.get("name")?,
                created_at: node.get("created_at")?,
            }))
        } else {
            Ok(None)
        }
    }

    pub async fn list_persons(&self) -> anyhow::Result<Vec<PersonNode>> {
        let mut result = self
            .graph
            .execute(query("MATCH (p:Person) RETURN p ORDER BY p.created_at DESC"))
            .await?;

        let mut persons = Vec::new();
        while let Some(row) = result.next().await? {
            let node: neo4rs::Node = row.get("p")?;
            persons.push(PersonNode {
                id: node.get("id")?,
                name: node.get("name")?,
                created_at: node.get("created_at")?,
            });
        }
        Ok(persons)
    }

    pub async fn list_identities_for_person(&self, person_id: &str) -> anyhow::Result<Vec<IdentityNode>> {
        let mut result = self
            .graph
            .execute(
                query("MATCH (i:Identity)-[:IDENTIFIES]->(p:Person {id: $id}) RETURN i")
                    .param("id", person_id.to_string()),
            )
            .await?;

        let mut identities = Vec::new();
        while let Some(row) = result.next().await? {
            let node: neo4rs::Node = row.get("i")?;
            let document_type: String = node.get("document_type")?;
            let document_number: String = node.get("document_number")?;
            identities.push(IdentityNode {
                id: node.get("id")?,
                full_name: node.get("full_name")?,
                document_type: if document_type.is_empty() {
                    None
                } else {
                    Some(document_type)
                },
                document_number: if document_number.is_empty() {
                    None
                } else {
                    Some(document_number)
                },
                created_at: node.get("created_at")?,
            });
        }
        Ok(identities)
    }

    /// The Person this Identity already IDENTIFIES, if any.
    pub async fn get_person_for_identity(&self, identity_id: &str) -> anyhow::Result<Option<String>> {
        let mut result = self
            .graph
            .execute(
                query(
                    "MATCH (i:Identity {id: $id})-[:IDENTIFIES]->(p:Person)
                     RETURN p.id AS pid ORDER BY p.created_at ASC LIMIT 1",
                )
                .param("id", identity_id.to_string()),
            )
            .await?;

        if let Some(row) = result.next().await? {
            let pid: String = row.get("pid")?;
            Ok(Some(pid))
        } else {
            Ok(None)
        }
    }

    pub async fn attach_identity_to_person(
        &self,
        identity_id: &str,
        person_id: &str,
    ) -> anyhow::Result<()> {
        self.run(
            query(
                "MATCH (i:Identity {id: $i}) MATCH (p:Person {id: $p})
                 CREATE (i)-[:IDENTIFIES]->(p)",
            )
            .param("i", identity_id.to_string())
            .param("p", person_id.to_string()),
        )
        .await?;
        Ok(())
    }

    pub async fn link_face_records_to_person(
        &self,
        face_record_ids: &[String],
        person_id: &str,
    ) -> anyhow::Result<()> {
        if face_record_ids.is_empty() {
            return Ok(());
        }
        let ids: Vec<String> = face_record_ids.to_vec();
        self.run(
            query(
                "UNWIND $ids AS fr_id
                 MATCH (fr:FaceRecord {id: fr_id})
                 MATCH (p:Person {id: $p})
                 CREATE (fr)-[:face_record_of]->(p)",
            )
            .param("ids", ids)
            .param("p", person_id.to_string()),
        )
        .await?;
        Ok(())
    }

    pub async fn log_action(
        &self,
        action_type: &str,
        actor: &str,
        affects_ids: &[String],
        confidence: Option<f64>,
        notes: Option<&str>,
    ) -> anyhow::Result<()> {
        self.run(
            query(
                "CREATE (a:AnalystAction {
                    id: $id, action_type: $action_type, actor: $actor,
                    confidence: $confidence, notes: $notes, created_at: $created_at
                 })
                 WITH a
                 UNWIND $affects_ids AS affected_id
                 MATCH (n {id: affected_id})
                 CREATE (a)-[:AFFECTS]->(n)",
            )
            .param("id", Uuid::new_v4().to_string())
            .param("action_type", action_type.to_string())
            .param("actor", actor.to_string())
            .param("confidence", confidence)
            .param("notes", notes.map(String::from))
            .param("created_at", chrono::Utc::now().to_rfc3339())
            .param("affects_ids", affects_ids.to_vec()),
        )
        .await?;
        Ok(())
    }
}

// ─── Target-centric container structure (tid:target-centric) ───────────────

pub struct TargetSystemNode {
    pub id: String,
    pub name: String,
    pub description: Option<String>,
    pub created_at: String,
}

pub struct SituationNode {
    pub id: String,
    pub name: String,
    pub description: Option<String>,
    pub target_system_id: String,
    pub created_at: String,
}

pub struct IncidentNode {
    pub id: String,
    pub description: String,
    pub occurred_at: Option<String>,
    pub created_at: String,
}

pub struct ObservationNode {
    pub id: String,
    pub description: String,
    pub created_at: String,
}

fn opt_str(s: String) -> Option<String> {
    if s.is_empty() {
        None
    } else {
        Some(s)
    }
}

impl GraphService {
    pub async fn create_target_system(
        &self,
        name: &str,
        description: Option<&str>,
    ) -> anyhow::Result<String> {
        let id = Uuid::new_v4().to_string();
        self.run(
            query(
                "CREATE (t:TargetSystem {id: $id, name: $name, description: $description, created_at: $created_at})",
            )
            .param("id", id.clone())
            .param("name", name.to_string())
            .param("description", description.unwrap_or("").to_string())
            .param("created_at", chrono::Utc::now().to_rfc3339()),
        )
        .await?;
        Ok(id)
    }

    pub async fn list_target_systems(&self) -> anyhow::Result<Vec<TargetSystemNode>> {
        let mut result = self
            .graph
            .execute(query("MATCH (t:TargetSystem) RETURN t ORDER BY t.created_at DESC"))
            .await?;
        let mut out = Vec::new();
        while let Some(row) = result.next().await? {
            let node: neo4rs::Node = row.get("t")?;
            let description: String = node.get("description")?;
            out.push(TargetSystemNode {
                id: node.get("id")?,
                name: node.get("name")?,
                description: opt_str(description),
                created_at: node.get("created_at")?,
            });
        }
        Ok(out)
    }

    pub async fn get_target_system(&self, id: &str) -> anyhow::Result<Option<TargetSystemNode>> {
        let mut result = self
            .graph
            .execute(query("MATCH (t:TargetSystem {id: $id}) RETURN t").param("id", id.to_string()))
            .await?;
        if let Some(row) = result.next().await? {
            let node: neo4rs::Node = row.get("t")?;
            let description: String = node.get("description")?;
            Ok(Some(TargetSystemNode {
                id: id.to_string(),
                name: node.get("name")?,
                description: opt_str(description),
                created_at: node.get("created_at")?,
            }))
        } else {
            Ok(None)
        }
    }

    pub async fn create_situation(
        &self,
        target_system_id: &str,
        name: &str,
        description: Option<&str>,
    ) -> anyhow::Result<Option<String>> {
        let id = Uuid::new_v4().to_string();
        let mut result = self
            .graph
            .execute(
                query(
                    "MATCH (t:TargetSystem {id: $ts})
                     CREATE (s:Situation {id: $id, name: $name, description: $description,
                                          target_system_id: $ts, created_at: $created_at})
                     CREATE (s)-[:organized_under]->(t)
                     RETURN s",
                )
                .param("ts", target_system_id.to_string())
                .param("id", id.clone())
                .param("name", name.to_string())
                .param("description", description.unwrap_or("").to_string())
                .param("created_at", chrono::Utc::now().to_rfc3339()),
            )
            .await?;
        if result.next().await?.is_some() {
            Ok(Some(id))
        } else {
            Ok(None)
        }
    }

    pub async fn list_situations(&self, target_system_id: &str) -> anyhow::Result<Vec<SituationNode>> {
        let mut result = self
            .graph
            .execute(
                query("MATCH (s:Situation)-[:organized_under]->(t:TargetSystem {id: $id}) RETURN s ORDER BY s.created_at DESC")
                    .param("id", target_system_id.to_string()),
            )
            .await?;
        let mut out = Vec::new();
        while let Some(row) = result.next().await? {
            let node: neo4rs::Node = row.get("s")?;
            let description: String = node.get("description")?;
            out.push(SituationNode {
                id: node.get("id")?,
                name: node.get("name")?,
                description: opt_str(description),
                target_system_id: node.get("target_system_id")?,
                created_at: node.get("created_at")?,
            });
        }
        Ok(out)
    }

    pub async fn get_situation(&self, id: &str) -> anyhow::Result<Option<SituationNode>> {
        let mut result = self
            .graph
            .execute(query("MATCH (s:Situation {id: $id}) RETURN s").param("id", id.to_string()))
            .await?;
        if let Some(row) = result.next().await? {
            let node: neo4rs::Node = row.get("s")?;
            let description: String = node.get("description")?;
            Ok(Some(SituationNode {
                id: id.to_string(),
                name: node.get("name")?,
                description: opt_str(description),
                target_system_id: node.get("target_system_id")?,
                created_at: node.get("created_at")?,
            }))
        } else {
            Ok(None)
        }
    }

    pub async fn create_incident(
        &self,
        situation_id: &str,
        description: &str,
        occurred_at: Option<&str>,
    ) -> anyhow::Result<Option<String>> {
        let id = Uuid::new_v4().to_string();
        let mut result = self
            .graph
            .execute(
                query(
                    "MATCH (s:Situation {id: $situation})
                     CREATE (i:Incident {id: $id, description: $description, occurred_at: $occurred_at,
                                          situation_id: $situation, created_at: $created_at})
                     CREATE (s)-[:explains]->(i)
                     RETURN i",
                )
                .param("situation", situation_id.to_string())
                .param("id", id.clone())
                .param("description", description.to_string())
                .param("occurred_at", occurred_at.unwrap_or("").to_string())
                .param("created_at", chrono::Utc::now().to_rfc3339()),
            )
            .await?;
        if result.next().await?.is_some() {
            Ok(Some(id))
        } else {
            Ok(None)
        }
    }

    pub async fn list_incidents(&self, situation_id: &str) -> anyhow::Result<Vec<IncidentNode>> {
        let mut result = self
            .graph
            .execute(
                query("MATCH (s:Situation {id: $id})-[:explains]->(i:Incident) RETURN i ORDER BY i.created_at DESC")
                    .param("id", situation_id.to_string()),
            )
            .await?;
        let mut out = Vec::new();
        while let Some(row) = result.next().await? {
            let node: neo4rs::Node = row.get("i")?;
            let occurred_at: String = node.get("occurred_at")?;
            out.push(IncidentNode {
                id: node.get("id")?,
                description: node.get("description")?,
                occurred_at: opt_str(occurred_at),
                created_at: node.get("created_at")?,
            });
        }
        Ok(out)
    }

    pub async fn get_incident(&self, id: &str) -> anyhow::Result<Option<IncidentNode>> {
        let mut result = self
            .graph
            .execute(query("MATCH (i:Incident {id: $id}) RETURN i").param("id", id.to_string()))
            .await?;
        if let Some(row) = result.next().await? {
            let node: neo4rs::Node = row.get("i")?;
            let occurred_at: String = node.get("occurred_at")?;
            Ok(Some(IncidentNode {
                id: id.to_string(),
                description: node.get("description")?,
                occurred_at: opt_str(occurred_at),
                created_at: node.get("created_at")?,
            }))
        } else {
            Ok(None)
        }
    }

    pub async fn create_observation(&self, description: &str) -> anyhow::Result<String> {
        let id = Uuid::new_v4().to_string();
        self.run(
            query(
                "CREATE (o:Observation {id: $id, description: $description, created_at: $created_at})",
            )
            .param("id", id.clone())
            .param("description", description.to_string())
            .param("created_at", chrono::Utc::now().to_rfc3339()),
        )
        .await?;
        Ok(id)
    }

    pub async fn list_observations(&self, incident_id: &str) -> anyhow::Result<Vec<ObservationNode>> {
        let mut result = self
            .graph
            .execute(
                query("MATCH (i:Incident {id: $id})-[:has_observation]->(o:Observation) RETURN o ORDER BY o.created_at DESC")
                    .param("id", incident_id.to_string()),
            )
            .await?;
        let mut out = Vec::new();
        while let Some(row) = result.next().await? {
            let node: neo4rs::Node = row.get("o")?;
            out.push(ObservationNode {
                id: node.get("id")?,
                description: node.get("description")?,
                created_at: node.get("created_at")?,
            });
        }
        Ok(out)
    }

    pub async fn link_observation_to_incident(
        &self,
        observation_id: &str,
        incident_id: &str,
    ) -> anyhow::Result<bool> {
        let mut result = self
            .graph
            .execute(
                query(
                    "MATCH (i:Incident {id: $i}) MATCH (o:Observation {id: $o})
                     CREATE (i)-[:has_observation]->(o) RETURN i",
                )
                .param("i", incident_id.to_string())
                .param("o", observation_id.to_string()),
            )
            .await?;
        Ok(result.next().await?.is_some())
    }

    pub async fn link_observation_to_target_system(
        &self,
        observation_id: &str,
        target_system_id: &str,
    ) -> anyhow::Result<bool> {
        let mut result = self
            .graph
            .execute(
                query(
                    "MATCH (t:TargetSystem {id: $t}) MATCH (o:Observation {id: $o})
                     CREATE (o)-[:tracked_in]->(t) RETURN o",
                )
                .param("t", target_system_id.to_string())
                .param("o", observation_id.to_string()),
            )
            .await?;
        Ok(result.next().await?.is_some())
    }

    pub async fn link_observation_to_face_record(
        &self,
        observation_id: &str,
        face_record_id: &str,
    ) -> anyhow::Result<()> {
        self.run(
            query(
                "MATCH (o:Observation {id: $o}) MATCH (fr:FaceRecord {id: $fr})
                 CREATE (o)-[:produces]->(fr)",
            )
            .param("o", observation_id.to_string())
            .param("fr", face_record_id.to_string()),
        )
        .await?;
        Ok(())
    }
}

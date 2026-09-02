// Phase 0 spike 1: can neo4rs (Bolt 4.x) connect to Memgraph, create the
// face-record graph, and read it back? This is the biggest Rust ecosystem
// risk in the transition plan.
use neo4rs::{query, Graph};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let uri = "bolt://localhost:7687";
    println!("connecting to {uri} ...");
    let graph = Graph::new(uri, "", "")?;
    println!("connected.");

    // Clean slate.
    graph
        .run(query("MATCH (n) DETACH DELETE n"))
        .await?;
    println!("cleared graph.");

    // Seed a minimal face-record instance graph.
    graph
        .run(query(
            "MERGE (p:Person {id: 'p1'}) SET p.name = 'Mark'
             CREATE (r:FaceRecord {id: 'r1', source: 'surveillance'})
             CREATE (img:FaceImage {id: 'img1', kind: 'SurveillanceFaceImage'})
             CREATE (emb:FaceEmbedding {id: 'emb1', vector: [0.1, 0.2, 0.3]})
             CREATE (f:Face {id: 'face1'})
             CREATE (p)<-[:face_record_of]-(r)
             CREATE (r)-[:has_face_image]->(img)
             CREATE (r)-[:has_face_embedding]->(emb)
             CREATE (img)-[:depicts]->(f)
             CREATE (emb)-[:derived_from]->(img)",
        ))
        .await?;
    println!("seeded Person/FaceRecord/FaceImage/FaceEmbedding graph.");

    // Read back: traverse the core link.
    let mut result = graph
        .execute(query(
            "MATCH (r:FaceRecord)-[:face_record_of]->(p:Person)
             RETURN p.name AS name, r.source AS source",
        ))
        .await?;
    while let Some(row) = result.next().await? {
        let name: String = row.get("name")?;
        let source: String = row.get("source")?;
        println!("face_record_of -> Person(name={name}, record_source={source})");
    }

    // Edge/type sanity: count relation types.
    let mut result = graph
        .execute(query(
            "MATCH (p:Person)<-[e:face_record_of]-() RETURN type(e) AS t, count(*) AS n",
        ))
        .await?;
    while let Some(row) = result.next().await? {
        let t: String = row.get("t")?;
        let n: i64 = row.get("n")?;
        println!("relation {t} x {n}");
    }

    // The resolution pattern: for a query embedding, find the Person via its
    // FaceRecords' embeddings (here approximated by direct property match).
    let mut result = graph
        .execute(query(
            "MATCH (emb:FaceEmbedding)-[:derived_from]->(:FaceImage)
             OPTIONAL MATCH (:FaceRecord)-[:has_face_embedding]->(emb)-[:face_record_of]->(p:Person)
             RETURN emb.id AS emb_id, p.name AS person
             LIMIT 10",
        ))
        .await?;
    while let Some(row) = result.next().await? {
        let emb_id: String = row.get("emb_id")?;
        let person: Option<String> = row.get("person")?;
        println!("embedding {emb_id} -> person {person:?}");
    }

    println!("OK: neo4rs <-> Memgraph round-trip works.");
    Ok(())
}

// Phase 0 spike 2: sqlx + pgvector — create the extension, insert 512-d
// embeddings, build an HNSW cosine index, and run the nearest-neighbour
// search that face resolution will depend on.
use pgvector::Vector;
use sqlx::postgres::PgPoolOptions;
use sqlx::Row;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let url = "postgres://postgres:password@localhost:5432/trackid";
    let pool = PgPoolOptions::new()
        .max_connections(5)
        .connect(url)
        .await?;
    println!("connected to Postgres.");

    sqlx::query("CREATE EXTENSION IF NOT EXISTS vector")
        .execute(&pool)
        .await?;
    sqlx::query("DROP TABLE IF EXISTS spike_embeddings")
        .execute(&pool)
        .await?;
    sqlx::query("CREATE TABLE spike_embeddings (id serial PRIMARY KEY, embedding vector(512))")
        .execute(&pool)
        .await?;
    println!("created table spike_embeddings (vector(512)).");

    // Seed orthogonal basis-ish vectors: v[i] is 1.0 in dim i, 0 elsewhere.
    // Non-collinear so cosine distance is a meaningful, varied signal.
    for i in 0..5 {
        let mut values = vec![0.0f32; 512];
        values[i] = 1.0;
        let v = Vector::from(values);
        sqlx::query("INSERT INTO spike_embeddings (embedding) VALUES ($1)")
            .bind(v)
            .execute(&pool)
            .await?;
    }
    println!("inserted 5 orthogonal vectors.");

    // HNSW index with cosine ops — same shape as the Python models'
    // ix_resolution_embeddings_embedding_hnsw.
    sqlx::query(
        "CREATE INDEX ON spike_embeddings USING hnsw (embedding vector_cosine_ops)",
    )
    .execute(&pool)
    .await?;
    println!("built HNSW cosine index.");

    // Nearest-neighbour search for a query that exactly matches v[0].
    let query_vec = Vector::from({
        let mut values = vec![0.0f32; 512];
        values[0] = 1.0;
        values
    });
    let rows = sqlx::query(
        "SELECT id, embedding <=> $1 AS distance
         FROM spike_embeddings
         ORDER BY embedding <=> $1
         LIMIT 3",
    )
    .bind(query_vec)
    .fetch_all(&pool)
    .await?;

    for row in rows {
        let id: i32 = row.get("id");
        let distance: f64 = row.get("distance");
        println!("id={id} cosine_distance={distance:.6}");
    }

    println!("OK: sqlx + pgvector HNSW cosine search works.");
    Ok(())
}

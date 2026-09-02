//! Auth: argon2 password hashing, JWT signing/verification, and the axum
//! middleware that gates the protected routes.

use argon2::password_hash::phc::PasswordHash;
use argon2::{Argon2, PasswordHasher, PasswordVerifier};
use axum::extract::{Request, State};
use axum::http::header::AUTHORIZATION;
use axum::http::StatusCode;
use axum::middleware::Next;
use axum::response::Response;
use jsonwebtoken::{decode, encode, DecodingKey, EncodingKey, Header, Validation};
use serde::{Deserialize, Serialize};

use crate::error::ApiError;

#[derive(Clone)]
pub struct AuthState {
    pub secret: String,
    pub token_hours: i64,
}

impl AuthState {
    pub fn new(secret: String, token_hours: i64) -> Self {
        Self {
            secret,
            token_hours,
        }
    }
}

/// The authenticated principal, inserted into request extensions by the
/// middleware for handlers that need to attribute an action to a user.
#[derive(Clone, Debug)]
pub struct AuthUser {
    pub user_id: String,
    pub username: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct Claims {
    pub sub: String,
    pub username: String,
    pub exp: usize,
    pub iat: usize,
}

pub fn hash_password(password: &str) -> anyhow::Result<String> {
    Ok(Argon2::default().hash_password(password.as_bytes())?.to_string())
}

pub fn verify_password(password: &str, hash: &str) -> bool {
    let Ok(parsed) = hash.parse::<PasswordHash>() else {
        return false;
    };
    Argon2::default()
        .verify_password(password.as_bytes(), &parsed)
        .is_ok()
}

pub fn create_token(auth: &AuthState, user_id: &str, username: &str) -> anyhow::Result<String> {
    let now = chrono::Utc::now();
    let iat = now.timestamp() as usize;
    let exp = (now + chrono::Duration::hours(auth.token_hours)).timestamp() as usize;
    let claims = Claims {
        sub: user_id.to_string(),
        username: username.to_string(),
        exp,
        iat,
    };
    Ok(encode(
        &Header::default(),
        &claims,
        &EncodingKey::from_secret(auth.secret.as_bytes()),
    )?)
}

pub fn verify_token(auth: &AuthState, token: &str) -> anyhow::Result<Claims> {
    let data = decode::<Claims>(
        token,
        &DecodingKey::from_secret(auth.secret.as_bytes()),
        &Validation::default(),
    )?;
    Ok(data.claims)
}

pub async fn auth_middleware(
    State(auth): State<AuthState>,
    mut req: Request,
    next: Next,
) -> Result<Response, ApiError> {
    let token = req
        .headers()
        .get(AUTHORIZATION)
        .and_then(|v| v.to_str().ok())
        .and_then(|v| v.strip_prefix("Bearer "))
        .ok_or_else(|| ApiError::new(StatusCode::UNAUTHORIZED, "missing or malformed Authorization header"))?;

    let claims = verify_token(&auth, token)
        .map_err(|_| ApiError::new(StatusCode::UNAUTHORIZED, "invalid or expired token"))?;

    req.extensions_mut().insert(AuthUser {
        user_id: claims.sub.clone(),
        username: claims.username.clone(),
    });

    Ok(next.run(req).await)
}

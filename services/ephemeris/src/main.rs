//! Standalone, publishable Swiss Ephemeris HTTP service. This crate must not
//! depend on or import the proprietary trading/scoring application.
mod protocol;
use axum::{
    Json, Router,
    extract::{DefaultBodyLimit, Request, State},
    http::StatusCode,
    middleware::{self, Next},
    response::{IntoResponse, Response},
    routing::{get, post},
};
use protocol::{Position, Request as Calculation, Response as CalculationResult};
use secrecy::{ExposeSecret, SecretString};
use sha2::{Digest, Sha256};
use std::{
    sync::Arc,
    time::{Duration, Instant},
};
use subtle::ConstantTimeEq;
use tokio::sync::{Mutex, Semaphore};
#[derive(Clone)]
struct App {
    secret: Arc<SecretString>,
    cpu: Arc<Semaphore>,
    quota: Arc<Mutex<(Instant, u32)>>,
}
async fn auth(State(app): State<App>, req: Request, next: Next) -> Response {
    let supplied = req
        .headers()
        .get("x-ephemeris-secret")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("");
    if req.headers().get_all("x-ephemeris-secret").iter().count() != 1
        || !bool::from(
            Sha256::digest(supplied.as_bytes())
                .ct_eq(&Sha256::digest(app.secret.expose_secret().as_bytes())),
        )
    {
        return (
            StatusCode::UNAUTHORIZED,
            Json(serde_json::json!({"error":"Unauthorized"})),
        )
            .into_response();
    }
    {
        let mut quota = app.quota.lock().await;
        if quota.0.elapsed() >= Duration::from_secs(60) {
            *quota = (Instant::now(), 0)
        }
        if quota.1 >= 6000 {
            return StatusCode::TOO_MANY_REQUESTS.into_response();
        }
        quota.1 += 1;
    }
    let mut response = next.run(req).await;
    response
        .headers_mut()
        .insert("cache-control", "private, no-store".parse().unwrap());
    response
}
async fn calculate(
    State(app): State<App>,
    Json(req): Json<Calculation>,
) -> Result<Json<CalculationResult>, StatusCode> {
    let date =
        chrono::DateTime::from_timestamp_millis(req.timestamp_ms).ok_or(StatusCode::BAD_REQUEST)?;
    // Bundled ephemeris covers 1800–2399. Fail explicitly outside that range.
    use chrono::Datelike;
    if !(1800..2400).contains(&date.year())
        || req.latitude.is_some() != req.longitude.is_some()
        || req
            .latitude
            .is_some_and(|x| !x.is_finite() || x.abs() > 90.)
        || req
            .longitude
            .is_some_and(|x| !x.is_finite() || x.abs() > 180.)
    {
        return Err(StatusCode::BAD_REQUEST);
    }
    let permit = app
        .cpu
        .clone()
        .try_acquire_owned()
        .map_err(|_| StatusCode::TOO_MANY_REQUESTS)?;
    tokio::task::spawn_blocking(move || {
        let _permit = permit;
        let jd = req.timestamp_ms as f64 / 86_400_000. + 2_440_587.5;
        let bodies = [
            sweph::Body::Sun,
            sweph::Body::Moon,
            sweph::Body::Mercury,
            sweph::Body::Venus,
            sweph::Body::Mars,
            sweph::Body::Jupiter,
            sweph::Body::Saturn,
            sweph::Body::Uranus,
            sweph::Body::Neptune,
            sweph::Body::Pluto,
        ];
        let mut planets = Vec::new();
        for body in bodies {
            let p = sweph::calc_with(jd, body, sweph::Flags::SWISS)
                .map_err(|_| StatusCode::UNPROCESSABLE_ENTITY)?;
            let eq = sweph::calc_with(jd, body, sweph::Flags::SWISS | sweph::Flags::EQUATORIAL)
                .map_err(|_| StatusCode::UNPROCESSABLE_ENTITY)?;
            planets.push(Position {
                longitude: p.longitude,
                latitude: p.latitude,
                speed_longitude: p.speed_longitude,
                speed_latitude: p.speed_latitude,
                declination: eq.latitude,
                declination_speed: eq.speed_latitude,
            });
        }
        let true_node = sweph::calc_with(jd, sweph::Body::TrueNode, sweph::Flags::SWISS)
            .map_err(|_| StatusCode::UNPROCESSABLE_ENTITY)?
            .longitude;
        let house_cusps = match (req.latitude, req.longitude) {
            (Some(lat), Some(lon)) if lat.abs() <= 66. => {
                sweph::houses(jd, lat, lon, sweph::HouseSystem::Placidus)
                    .ok()
                    .map(|h| h.cusps.to_vec())
            }
            _ => None,
        };
        Ok(Json(CalculationResult {
            planets: planets
                .try_into()
                .map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?,
            true_node,
            house_cusps,
        }))
    })
    .await
    .map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?
}
#[tokio::main]
async fn main() {
    if let Err(()) = run().await {
        eprintln!("Ephemeris service failed: verify required configuration and data files");
        std::process::exit(1);
    }
}
async fn run() -> Result<(), ()> {
    let secret = SecretString::from(std::env::var("EPHEMERIS_SERVICE_SECRET").map_err(|_| ())?);
    if secret.expose_secret().len() < 32 {
        return Err(());
    }
    let data = std::env::var("ASTRADIA_EPHEMERIS_PATH")
        .unwrap_or_else(|_| format!("{}/data/ephemeris", env!("CARGO_MANIFEST_DIR")));
    for file in ["sepl_18.se1", "semo_18.se1"] {
        if !std::path::Path::new(&data).join(file).is_file() {
            return Err(());
        }
    }
    sweph::set_ephe_path(&data);
    let app = App {
        secret: Arc::new(secret),
        cpu: Arc::new(Semaphore::new(2)),
        quota: Arc::new(Mutex::new((Instant::now(), 0))),
    };
    let routes = Router::new()
        .route("/v1/ephemeris", post(calculate))
        .route_layer(middleware::from_fn_with_state(app.clone(), auth))
        .route(
            "/health",
            get(|| async { Json(serde_json::json!({"ok":true})) }),
        )
        .route(
            "/source",
            get(|| async {
                (
                    [
                        ("content-type", "application/gzip"),
                        (
                            "content-disposition",
                            "attachment; filename=astradia-ephemeris-source.tar.gz",
                        ),
                    ],
                    include_bytes!(concat!(env!("OUT_DIR"), "/source.tar.gz")).as_slice(),
                )
            }),
        )
        .layer(DefaultBodyLimit::max(4096))
        .with_state(app);
    let addr = std::env::var("EPHEMERIS_BIND").unwrap_or_else(|_| "127.0.0.1:8091".into());
    let socket = tokio::net::TcpListener::bind(addr).await.map_err(|_| ())?;
    axum::serve(socket, routes)
        .with_graceful_shutdown(async {
            let _ = tokio::signal::ctrl_c().await;
        })
        .await
        .map_err(|_| ())
}

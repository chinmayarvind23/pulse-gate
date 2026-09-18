mod model;

use std::{collections::HashMap, env, sync::Arc, time::Duration};
use axum::{extract::State, http::StatusCode, routing::{get, post}, Json, Router};
use model::{PaymentEvent, RiskDecision};
use redis::{aio::MultiplexedConnection, streams::StreamReadReply, AsyncCommands, RedisResult};
use serde::Serialize;
use tokio::sync::Mutex;
use tracing::{error, info};

#[derive(Clone)]
struct AppState {
    redis: Arc<Mutex<MultiplexedConnection>>,
    result_stream: String,
}

#[derive(Serialize)]
struct Health { status: &'static str }

/// The worker runs stream consumption and a small diagnostic HTTP server in one
/// process. `/score` exists for reproducible offline evaluation against exactly the
/// same scoring function used by the stream consumer. It is not part of the payment
/// ingress path and can be network-restricted in production.
#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    tracing_subscriber::fmt().with_env_filter(tracing_subscriber::EnvFilter::from_default_env()).init();
    let redis_addr = env::var("PULSEGATE_REDIS_ADDR").unwrap_or_else(|_| "redis:6379".into());
    let stream = env::var("PULSEGATE_STREAM").unwrap_or_else(|_| "payment_events".into());
    let result_stream = env::var("PULSEGATE_RESULT_STREAM").unwrap_or_else(|_| "risk_decisions".into());
    let group = env::var("PULSEGATE_WORKER_GROUP").unwrap_or_else(|_| "risk-workers".into());
    let consumer = env::var("PULSEGATE_WORKER_CONSUMER").unwrap_or_else(|_| format!("worker-{}", std::process::id()));
    let http_addr = env::var("PULSEGATE_WORKER_HTTP_ADDR").unwrap_or_else(|_| "0.0.0.0:8081".into());

    let client = redis::Client::open(format!("redis://{redis_addr}/"))?;
    let connection = client.get_multiplexed_async_connection().await?;
    let state = AppState { redis: Arc::new(Mutex::new(connection)), result_stream };
    ensure_group(state.redis.clone(), &stream, &group).await;

    let consume_state = state.clone();
    let consume_stream = stream.clone();
    tokio::spawn(async move { consume_loop(consume_state, consume_stream, group, consumer).await; });

    let app = Router::new()
        .route("/healthz", get(|| async { Json(Health{status:"ok"}) }))
        .route("/score", post(score_endpoint))
        .with_state(state);
    let listener = tokio::net::TcpListener::bind(&http_addr).await?;
    info!(%http_addr, "risk worker diagnostic server listening");
    axum::serve(listener, app).await?;
    Ok(())
}

async fn score_endpoint(State(_state): State<AppState>, Json(event): Json<PaymentEvent>) -> Result<Json<RiskDecision>, StatusCode> {
    if event.event_id.is_empty() || event.amount_cents < 0 || !(0..=23).contains(&event.hour_utc) { return Err(StatusCode::BAD_REQUEST); }
    Ok(Json(model::score(&event)))
}

/// ensure_group is idempotent so worker replicas may start concurrently. A failed
/// group-creation attempt is tolerated when the group already exists; subsequent reads
/// are the real readiness signal.
async fn ensure_group(redis: Arc<Mutex<MultiplexedConnection>>, stream: &str, group: &str) {
    let mut conn = redis.lock().await;
    let _: RedisResult<String> = redis::cmd("XGROUP").arg("CREATE").arg(stream).arg(group).arg("0").arg("MKSTREAM").query_async(&mut *conn).await;
}

/// consume_loop uses Redis consumer groups so each stream entry is assigned to one
/// worker. XACK occurs only after the decision has been persisted to the result stream.
/// A production version would add pending-entry reclamation and a durable decision
/// store before claiming exactly-once business effects.
async fn consume_loop(state: AppState, stream: String, group: String, consumer: String) {
    loop {
        let reply = {
            let mut conn = state.redis.lock().await;
            redis::cmd("XREADGROUP")
                .arg("GROUP").arg(&group).arg(&consumer)
                .arg("COUNT").arg(64)
                .arg("BLOCK").arg(1000)
                .arg("STREAMS").arg(&stream).arg(">")
                .query_async::<StreamReadReply>(&mut *conn).await
        };
        match reply {
            Ok(reply) => {
                for key in reply.keys {
                    for id in key.ids {
                        if let Err(err) = process_one(&state, &stream, &group, &id.id, id.map).await { error!(%err, stream_id=%id.id, "event processing failed"); }
                    }
                }
            }
            Err(err) => { error!(%err, "stream read failed"); tokio::time::sleep(Duration::from_millis(250)).await; }
        }
    }
}

async fn process_one(state: &AppState, stream: &str, group: &str, stream_id: &str, fields: HashMap<String, redis::Value>) -> Result<(), Box<dyn std::error::Error>> {
    let payload: String = redis::from_redis_value(fields.get("payload").ok_or("missing payload")?)?;
    let event: PaymentEvent = serde_json::from_str(&payload)?;
    let decision = model::score(&event);
    let encoded = serde_json::to_string(&decision)?;
    let mut conn = state.redis.lock().await;
    let _: String = conn.xadd(&state.result_stream, "*", &[ ("event_id", decision.event_id.as_str()), ("decision", encoded.as_str()) ]).await?;
    let _: i64 = conn.xack(stream, group, &[stream_id]).await?;
    Ok(())
}

use axum::{
    extract::{DefaultBodyLimit, State},
    http::{header, StatusCode},
    routing::{get, post},
    Json, Router,
};
use hookguard_risk_worker::model::{self, PaymentEvent, RiskDecision};
use redis::{
    aio::{ConnectionManager, ConnectionManagerConfig},
    streams::{StreamAutoClaimReply, StreamId, StreamReadReply},
    AsyncCommands,
};
use std::{
    env,
    sync::{
        atomic::{AtomicBool, AtomicU64, Ordering},
        Arc,
    },
    time::{Duration, Instant},
};
use tokio::sync::{watch, RwLock};
use tracing::{error, info};

#[derive(Clone)]
struct AppState {
    redis: Arc<RwLock<ConnectionManager>>,
    client: redis::Client,
    config: Arc<Config>,
    stats: Arc<Stats>,
}
struct Config {
    stream: String,
    result: String,
    group: String,
    consumer: String,
    ttl: u64,
    reclaim_ms: u64,
    batch: usize,
}
#[derive(Default)]
struct Stats {
    ready: AtomicBool,
    decisions: AtomicU64,
    duplicates: AtomicU64,
    dead: AtomicU64,
    errors: AtomicU64,
    reclaimed: AtomicU64,
}

// Publish, mark and acknowledge under Redis isolation. A failed append removes its
// marker. Redelivery after a lost response observes the marker and cannot republish.
const FINISH: &str = r#"
local ok, result = pcall(function()
local input=KEYS[1]
local output=KEYS[2]
local done=KEYS[3]
local old=redis.call('GET',done)
if not old then
 local t=redis.call('TYPE',output).ok
 if t~='none' and t~='stream' then error('output must be stream') end
 redis.call('SET',done,'1','EX',ARGV[4])
 local id=redis.pcall('XADD',output,'MAXLEN','~',1000000,'*','event_id',ARGV[3],'decision',ARGV[5])
 if type(id)=='table' and id.err then redis.call('DEL',done);error(id.err) end
end
redis.call('XACK',input,ARGV[1],ARGV[2])
redis.call('XDEL',input,ARGV[2])
if old then return 0 else return 1 end
end)
if not ok then return {-1,tostring(result)} end
return {result,''}
"#;

// setting supplies process defaults without leaking configuration secrets.
fn setting(name: &str, default: &str) -> String {
    env::var(name).unwrap_or_else(|_| default.into())
}
// positive rejects unbounded or disabled work limits at startup.
fn positive(name: &str, default: u64) -> Result<u64, Box<dyn std::error::Error>> {
    let v = setting(name, &default.to_string()).parse::<u64>()?;
    if v == 0 {
        return Err(format!("{name} must be positive").into());
    };
    Ok(v)
}

#[tokio::main]
// main owns process lifetime; scoring and consumption share one immutable model.
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    tracing_subscriber::fmt()
        .with_env_filter(tracing_subscriber::EnvFilter::from_default_env())
        .init();
    let args: Vec<String> = env::args().collect();
    if args
        .get(1)
        .is_some_and(|s| s == "--benchmark" || s == "--predict-jsonl")
    {
        return offline(&args);
    }
    model::parameters();
    let config = Arc::new(Config {
        stream: setting("HOOKGUARD_STREAM", "payment_events"),
        result: setting("HOOKGUARD_RESULT_STREAM", "risk_decisions"),
        group: setting("HOOKGUARD_WORKER_GROUP", "risk-workers"),
        consumer: setting(
            "HOOKGUARD_WORKER_CONSUMER",
            &format!("{}-{}", setting("HOSTNAME", "worker"), std::process::id()),
        ),
        ttl: positive("HOOKGUARD_IDEMPOTENCY_TTL_SECONDS", 86400)?,
        reclaim_ms: positive("HOOKGUARD_RECLAIM_IDLE_MS", 5000)?,
        batch: positive("HOOKGUARD_WORKER_BATCH", 256)?.min(1024) as usize,
    });
    if config.stream == config.result {
        return Err("input and result streams must differ".into());
    }
    let client = redis::Client::open(format!(
        "redis://{}/",
        setting("HOOKGUARD_REDIS_ADDR", "redis:6379")
    ))?;
    // Socket deadlines let the manager detect dead connections after pod replacement.
    let redis_config = ConnectionManagerConfig::new()
        .set_connection_timeout(Duration::from_secs(2))
        .set_response_timeout(Duration::from_secs(2))
        .set_number_of_retries(2);
    let writer = ConnectionManager::new_with_config(client.clone(), redis_config.clone()).await?;
    let reader = ConnectionManager::new_with_config(client.clone(), redis_config).await?;
    let state = AppState {
        redis: Arc::new(RwLock::new(writer)),
        client,
        config,
        stats: Arc::new(Stats::default()),
    };
    ensure_group(&state).await?;
    let (stop_tx, stop_rx) = watch::channel(false);
    let task = tokio::spawn(consume(state.clone(), reader, stop_rx));
    let app = Router::new()
        .route("/healthz", get(|| async { "ok" }))
        .route("/readyz", get(ready))
        .route("/metrics", get(metrics))
        .route("/score", post(score_endpoint))
        .layer(DefaultBodyLimit::max(65536))
        .with_state(state);
    let addr = setting("HOOKGUARD_WORKER_HTTP_ADDR", "0.0.0.0:8081");
    let listener = tokio::net::TcpListener::bind(&addr).await?;
    info!(%addr,"worker listening");
    axum::serve(listener, app)
        .with_graceful_shutdown(async move {
            shutdown_signal().await;
            let _ = stop_tx.send(true);
        })
        .await?;
    let _ = tokio::time::timeout(Duration::from_secs(5), task).await;
    Ok(())
}
// shutdown_signal handles orchestrator SIGTERM as well as an interactive stop.
async fn shutdown_signal() {
    #[cfg(unix)]
    {
        let mut term = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
            .expect("SIGTERM");
        tokio::select! {_=tokio::signal::ctrl_c()=>{},_=term.recv()=>{}}
    }
    #[cfg(not(unix))]
    {
        let _ = tokio::signal::ctrl_c().await;
    }
}
// ready requires both a successful consumer cycle and a reachable Redis dependency.
async fn ready(State(s): State<AppState>) -> StatusCode {
    let mut c = s.redis.read().await.clone();
    let ping = tokio::time::timeout(
        Duration::from_millis(300),
        redis::cmd("PING").query_async::<String>(&mut c),
    )
    .await;
    if s.stats.ready.load(Ordering::Relaxed) && matches!(ping, Ok(Ok(_))) {
        StatusCode::OK
    } else {
        StatusCode::SERVICE_UNAVAILABLE
    }
}
// score_endpoint uses the production scoring path without publishing a decision.
async fn score_endpoint(Json(event): Json<PaymentEvent>) -> Result<Json<RiskDecision>, StatusCode> {
    if !event.valid() {
        return Err(StatusCode::BAD_REQUEST);
    };
    Ok(Json(model::score(&event)))
}
// metrics exports bounded counters and omits queue values when Redis cannot answer.
async fn metrics(State(s): State<AppState>) -> ([(header::HeaderName, &'static str); 1], String) {
    let mut c = s.redis.read().await.clone();
    let queued = tokio::time::timeout(
        Duration::from_millis(300),
        c.xlen::<_, u64>(&s.config.stream),
    )
    .await;
    let mut body=format!("# TYPE hookguard_worker_decisions_total counter\nhookguard_worker_decisions_total {}\n# TYPE hookguard_worker_duplicates_total counter\nhookguard_worker_duplicates_total {}\n# TYPE hookguard_worker_errors_total counter\nhookguard_worker_errors_total {}\n# TYPE hookguard_worker_reclaimed_total counter\nhookguard_worker_reclaimed_total {}\n# TYPE hookguard_worker_dead_letters_total counter\nhookguard_worker_dead_letters_total {}\n# TYPE hookguard_worker_ready gauge\nhookguard_worker_ready {}\n",s.stats.decisions.load(Ordering::Relaxed),s.stats.duplicates.load(Ordering::Relaxed),s.stats.errors.load(Ordering::Relaxed),s.stats.reclaimed.load(Ordering::Relaxed),s.stats.dead.load(Ordering::Relaxed),u8::from(s.stats.ready.load(Ordering::Relaxed)));
    if let Ok(Ok(n)) = queued {
        body +=
            &format!("# TYPE hookguard_queue_outstanding gauge\nhookguard_queue_outstanding {n}\n")
    }
    ([(header::CONTENT_TYPE, "text/plain; version=0.0.4")], body)
}
// ensure_group tolerates only an existing group; other startup failures stay visible.
async fn ensure_group(s: &AppState) -> redis::RedisResult<()> {
    tokio::time::timeout(Duration::from_secs(2), ensure_group_inner(s))
        .await
        .map_err(|_| {
            redis::RedisError::from((redis::ErrorKind::IoError, "group creation timed out"))
        })?
}

// ensure_group_inner keeps BUSYGROUP handling inside the dependency deadline.
async fn ensure_group_inner(s: &AppState) -> redis::RedisResult<()> {
    let mut c = s.redis.read().await.clone();
    let result = redis::cmd("XGROUP")
        .arg("CREATE")
        .arg(&s.config.stream)
        .arg(&s.config.group)
        .arg("0")
        .arg("MKSTREAM")
        .query_async::<String>(&mut c)
        .await;
    match result {
        Ok(_) => Ok(()),
        Err(e) if e.code() == Some("BUSYGROUP") => Ok(()),
        Err(e) => Err(e),
    }
}
// consume bounds batch memory and reclaims abandoned deliveries before reading new work.
async fn consume(s: AppState, mut reader: ConnectionManager, stop: watch::Receiver<bool>) {
    let mut cursor = "0-0".to_string();
    let mut last_claim = Instant::now() - Duration::from_secs(2);
    while !*stop.borrow() {
        let cycle = async {
            if last_claim.elapsed() >= Duration::from_secs(1) {
                let reclaimed = redis::cmd("XAUTOCLAIM")
                    .arg(&s.config.stream)
                    .arg(&s.config.group)
                    .arg(&s.config.consumer)
                    .arg(s.config.reclaim_ms)
                    .arg(&cursor)
                    .arg("COUNT")
                    .arg(s.config.batch)
                    .query_async::<StreamAutoClaimReply>(&mut reader)
                    .await?;
                cursor = reclaimed.next_stream_id;
                s.stats
                    .reclaimed
                    .fetch_add(reclaimed.claimed.len() as u64, Ordering::Relaxed);
                process_batch(&s, reclaimed.claimed).await?;
                last_claim = Instant::now();
            }
            let batch = redis::cmd("XREADGROUP")
                .arg("GROUP")
                .arg(&s.config.group)
                .arg(&s.config.consumer)
                .arg("COUNT")
                .arg(s.config.batch)
                .arg("BLOCK")
                .arg(500)
                .arg("STREAMS")
                .arg(&s.config.stream)
                .arg(">")
                .query_async::<StreamReadReply>(&mut reader)
                .await?;
            for key in batch.keys {
                process_batch(&s, key.ids).await?
            }
            Ok::<(), redis::RedisError>(())
        };
        match tokio::time::timeout(Duration::from_secs(4), cycle).await {
            Ok(Ok(())) => {
                s.stats.ready.store(true, Ordering::Relaxed);
            }
            other => {
                s.stats.ready.store(false, Ordering::Relaxed);
                s.stats.errors.fetch_add(1, Ordering::Relaxed);
                error!(error=?other,"consumer cycle failed");
                let transport_error = match &other {
                    Err(_) => true,
                    Ok(Err(e)) => e.is_io_error(),
                    _ => false,
                };
                if transport_error {
                    // A Kubernetes service can blackhole an old socket without FIN/RST.
                    // redis-rs retries response timeouts on that same socket, so replace both.
                    let replacement =
                        tokio::time::timeout(Duration::from_secs(5), reconnect(&s)).await;
                    if let Ok(Ok((new_reader, new_writer))) = replacement {
                        reader = new_reader;
                        *s.redis.write().await = new_writer;
                    }
                }
                let _ = ensure_group(&s).await;
                tokio::time::sleep(Duration::from_millis(250)).await;
            }
        }
    }
    s.stats.ready.store(false, Ordering::Relaxed);
}
// reconnect abandons stale service sockets while keeping readiness on the new writer.
async fn reconnect(s: &AppState) -> redis::RedisResult<(ConnectionManager, ConnectionManager)> {
    let config = ConnectionManagerConfig::new()
        .set_connection_timeout(Duration::from_secs(2))
        .set_response_timeout(Duration::from_secs(2))
        .set_number_of_retries(1);
    tokio::try_join!(
        ConnectionManager::new_with_config(s.client.clone(), config.clone()),
        ConnectionManager::new_with_config(s.client.clone(), config)
    )
}

// process_batch pipelines isolated transitions; partial failures leave unacknowledged work retryable.
async fn process_batch(s: &AppState, entries: Vec<StreamId>) -> redis::RedisResult<()> {
    if entries.is_empty() {
        return Ok(());
    };
    let mut pipe = redis::pipe();
    let mut dead = Vec::with_capacity(entries.len());
    for entry in &entries {
        let payload = entry.get::<String>("payload");
        let event = payload
            .as_deref()
            .and_then(|p| serde_json::from_str::<PaymentEvent>(p).ok())
            .filter(PaymentEvent::valid);
        let (id, encoded, output, key, is_dead) = match event {
            Some(event) => {
                let decision = model::score(&event);
                let id = event.event_id;
                let key = format!("{}:completed:{}", s.config.stream, id);
                (
                    id,
                    serde_json::to_string(&decision).expect("finite score"),
                    s.config.result.clone(),
                    key,
                    false,
                )
            }
            None => {
                let id = entry
                    .get::<String>("event_id")
                    .unwrap_or_else(|| entry.id.clone());
                let encoded=serde_json::json!({"stream_id":entry.id,"event_id":id,"error":"invalid event schema"}).to_string();
                let key = format!("{}:rejected:{}", s.config.stream, entry.id);
                (id, encoded, format!("{}:dead", s.config.result), key, true)
            }
        };
        pipe.cmd("EVAL")
            .arg(FINISH)
            .arg(3)
            .arg(&s.config.stream)
            .arg(output)
            .arg(key)
            .arg(&s.config.group)
            .arg(&entry.id)
            .arg(id)
            .arg(s.config.ttl)
            .arg(encoded);
        dead.push(is_dead);
    }
    let mut c = s.redis.read().await.clone();
    let results: Vec<(i64, String)> = pipe.query_async(&mut c).await?;
    let mut failed = false;
    for ((result, error), is_dead) in results.into_iter().zip(dead) {
        if result < 0 {
            failed = true;
            error!(%error, "completion failed; entry remains retryable");
            continue;
        }
        if result == 0 {
            s.stats.duplicates.fetch_add(1, Ordering::Relaxed);
        } else if is_dead {
            s.stats.dead.fetch_add(1, Ordering::Relaxed);
        } else {
            s.stats.decisions.fetch_add(1, Ordering::Relaxed);
        }
    }
    if failed {
        return Err(redis::RedisError::from((
            redis::ErrorKind::ResponseError,
            "one or more completions failed",
        )));
    }
    Ok(())
}
// offline exercises the serving function directly, separating arithmetic latency from HTTP.
fn offline(args: &[String]) -> Result<(), Box<dyn std::error::Error>> {
    let path = args.get(2).ok_or("dataset path required")?;
    let text = std::fs::read_to_string(path)?;
    let mut events = Vec::new();
    for line in text.lines() {
        let mut value: serde_json::Value = serde_json::from_str(line)?;
        value
            .as_object_mut()
            .ok_or("object required")?
            .remove("label");
        let event: PaymentEvent = serde_json::from_value(value)?;
        if !event.valid() {
            return Err("invalid dataset event".into());
        };
        events.push(event)
    }
    if events.is_empty() {
        return Err("empty dataset".into());
    }
    if args[1] == "--predict-jsonl" {
        for e in &events {
            println!("{}", serde_json::to_string(&model::score(e))?)
        }
        return Ok(());
    }
    let count = args
        .get(3)
        .map(|s| s.parse::<usize>())
        .transpose()?
        .unwrap_or(1000000);
    if count == 0 {
        return Err("count must be positive".into());
    }
    for e in &events {
        std::hint::black_box(model::score(e));
    }
    let mut times = Vec::with_capacity(count);
    let start = Instant::now();
    for i in 0..count {
        let now = Instant::now();
        std::hint::black_box(model::score(std::hint::black_box(
            &events[i % events.len()],
        )));
        times.push(now.elapsed().as_nanos() as u64)
    }
    let elapsed = start.elapsed().as_secs_f64();
    times.sort_unstable();
    println!(
        "{}",
        serde_json::json!({"samples":count,"elapsed_seconds":elapsed,"scores_per_second":count as f64/elapsed,"p50_ns":times[count/2],"p99_ns":times[(count*99/100).min(count-1)],"max_ns":times[count-1],"model_version":model::parameters().model_version,"scope":"in-process score including decision allocation; excludes JSON and network"})
    );
    Ok(())
}

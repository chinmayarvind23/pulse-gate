use serde::{Deserialize, Serialize};
use std::sync::OnceLock;

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct PaymentEvent {
    pub event_id: String,
    pub amount_cents: i64,
    pub currency: String,
    pub merchant_category: i32,
    pub country: String,
    pub card_present: bool,
    pub hour_utc: i32,
    pub velocity_5m: i32,
    pub velocity_1h: i32,
    pub prior_declines: i32,
}
impl PaymentEvent {
    /// valid mirrors ingress limits so direct diagnostics and poisoned streams fail closed.
    pub fn valid(&self) -> bool {
        !self.event_id.is_empty()
            && self.event_id.len() <= 128
            && self
                .event_id
                .bytes()
                .all(|c| c.is_ascii_alphanumeric() || b"_-.:".contains(&c))
            && self.amount_cents >= 0
            && self.currency.len() == 3
            && self.currency.bytes().all(|c| c.is_ascii_uppercase())
            && self.country.len() == 2
            && self.country.bytes().all(|c| c.is_ascii_uppercase())
            && (0..=9999).contains(&self.merchant_category)
            && (0..=23).contains(&self.hour_utc)
            && self.velocity_5m >= 0
            && self.velocity_1h >= 0
            && self.prior_declines >= 0
    }
}
#[derive(Debug, Clone, Serialize)]
pub struct RiskDecision {
    pub event_id: String,
    pub score: f64,
    pub risky: bool,
    pub model_version: &'static str,
}
#[derive(Deserialize)]
pub struct Model {
    pub model_version: String,
    pub weights: [f64; 7],
    pub intercept: f64,
    pub calibration_slope: f64,
    pub calibration_intercept: f64,
    pub threshold: f64,
}
static MODEL: OnceLock<Model> = OnceLock::new();
/// parameters parses the immutable serving artifact once, outside the scoring hot path.
pub fn parameters() -> &'static Model {
    MODEL.get_or_init(|| {
        serde_json::from_str(include_str!("../model/model.json")).expect("embedded model")
    })
}
/// features preserves the training order and clipping limits exactly.
pub fn features(e: &PaymentEvent) -> [f64; 7] {
    [
        (e.amount_cents as f64 / 100_000.0).clamp(0.0, 10.0),
        if e.hour_utc <= 5 || e.hour_utc >= 23 {
            1.0
        } else {
            0.0
        },
        if e.card_present { 0.0 } else { 1.0 },
        if e.country == "US" { 0.0 } else { 1.0 },
        (e.velocity_5m as f64 / 10.0).clamp(0.0, 10.0),
        (e.velocity_1h as f64 / 30.0).clamp(0.0, 10.0),
        (e.prior_declines as f64 / 5.0).clamp(0.0, 10.0),
    ]
}
/// score applies exported linear weights, sigmoid calibration, and the frozen threshold.
pub fn score(event: &PaymentEvent) -> RiskDecision {
    let m = parameters();
    let z = m.intercept
        + features(event)
            .iter()
            .zip(m.weights)
            .map(|(x, w)| x * w)
            .sum::<f64>();
    // Calibration is fitted on a disjoint split and exported with the transform.
    let probability = 1.0 / (1.0 + (-(z * m.calibration_slope + m.calibration_intercept)).exp());
    RiskDecision {
        event_id: event.event_id.clone(),
        score: probability,
        risky: probability >= m.threshold,
        model_version: m.model_version.as_str(),
    }
}
#[cfg(test)]
mod tests {
    use super::*;
    // event supplies the same complete contract used by the ingress.
    fn event() -> PaymentEvent {
        serde_json::from_str(r#"{"event_id":"test","amount_cents":2500,"currency":"USD","merchant_category":5812,"country":"US","card_present":true,"hour_utc":12,"velocity_5m":1,"velocity_1h":2,"prior_declines":0}"#).unwrap()
    }
    #[test]
    // schema exercises domain ranges that deserialization alone cannot enforce.
    fn schema() {
        let mut e = event();
        assert!(e.valid());
        e.velocity_5m = -1;
        assert!(!e.valid());
        e = event();
        e.country = "uS".into();
        assert!(!e.valid());
    }
    #[test]
    // monotonic checks the expected response to stronger risk features.
    fn monotonic() {
        let e = event();
        let normal = score(&e);
        let mut risk = e;
        risk.amount_cents = 900000;
        risk.prior_declines = 5;
        assert!(score(&risk).score > normal.score);
    }
    #[test]
    // finite_extremes protects sigmoid inference against extreme valid integers.
    fn finite_extremes() {
        let mut e = event();
        e.amount_cents = i64::MAX;
        e.velocity_5m = i32::MAX;
        let p = score(&e).score;
        assert!(p.is_finite() && (0.0..=1.0).contains(&p));
    }
    #[test]
    // exported_parameters rejects invalid serving configuration before deployment.
    fn exported_parameters() {
        let m = parameters();
        assert!(m.weights.iter().all(|v| v.is_finite()));
        assert!((0.0..1.0).contains(&m.threshold));
        assert!(m.calibration_slope > 0.0);
    }
}

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Deserialize)]
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

#[derive(Debug, Clone, Serialize)]
pub struct RiskDecision {
    pub event_id: String,
    pub score: f64,
    pub risky: bool,
    pub model_version: &'static str,
}

pub const MODEL_VERSION: &str = "logreg-v1";
pub const THRESHOLD: f64 = 0.58;

/// Score is a deliberately small logistic model so the entire inference path can be
/// inspected from first principles. The portfolio goal is to demonstrate feature
/// contracts, baseline comparison and serving behavior, not hide complexity behind a
/// large framework. A learned production model can replace these weights without
/// changing the stream contract.
pub fn score(event: &PaymentEvent) -> RiskDecision {
    let amount = (event.amount_cents as f64 / 100_000.0).clamp(0.0, 10.0);
    let night = if event.hour_utc <= 5 || event.hour_utc >= 23 { 1.0 } else { 0.0 };
    let not_present = if event.card_present { 0.0 } else { 1.0 };
    let foreign = if event.country.eq_ignore_ascii_case("US") { 0.0 } else { 1.0 };
    let velocity5 = (event.velocity_5m as f64 / 10.0).clamp(0.0, 10.0);
    let velocity1h = (event.velocity_1h as f64 / 30.0).clamp(0.0, 10.0);
    let declines = (event.prior_declines as f64 / 5.0).clamp(0.0, 10.0);

    let z = -3.20
        + 0.85 * amount
        + 0.55 * night
        + 0.70 * not_present
        + 0.65 * foreign
        + 1.05 * velocity5
        + 0.50 * velocity1h
        + 1.10 * declines;
    let probability = 1.0 / (1.0 + (-z).exp());
    RiskDecision { event_id: event.event_id.clone(), score: probability, risky: probability >= THRESHOLD, model_version: MODEL_VERSION }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn base_event() -> PaymentEvent {
        PaymentEvent { event_id:"evt".into(), amount_cents:2500, currency:"USD".into(), merchant_category:5812, country:"US".into(), card_present:true, hour_utc:12, velocity_5m:1, velocity_1h:2, prior_declines:0 }
    }

    #[test]
    fn risky_features_raise_score() {
        let normal = score(&base_event()).score;
        let mut risky = base_event();
        risky.amount_cents = 900_000; risky.country = "ZZ".into(); risky.card_present = false; risky.hour_utc = 2; risky.velocity_5m = 9; risky.velocity_1h = 20; risky.prior_declines = 4;
        assert!(score(&risky).score > normal);
    }

    #[test]
    fn score_is_bounded() {
        let s = score(&base_event()).score;
        assert!((0.0..=1.0).contains(&s));
    }
}

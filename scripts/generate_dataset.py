import json, random
from pathlib import Path

random.seed(7)
OUT=Path(__file__).resolve().parents[1]/'evals'/'data'/'risk_eval.jsonl'

def make(i:int)->dict:
    amount=random.randint(500,900000); country=random.choice(['US','US','US','CA','GB','ZZ']); present=random.random()>.35; hour=random.randrange(24); v5=random.randrange(12); v1=random.randrange(35); declines=random.randrange(6)
    # Synthetic label is generated from an independent rule with noise so the learned
    # scorer has a meaningful but imperfect benchmark. It must never be presented as a
    # real-world fraud metric.
    amount_x=min(amount/100000.0,10.0); night=1.0 if hour<=5 or hour>=23 else 0.0; not_present=0.0 if present else 1.0; foreign=0.0 if country=='US' else 1.0
    truth_logit=-3.35+0.90*amount_x+0.45*night+0.65*not_present+0.55*foreign+1.10*(v5/10.0)+0.45*(v1/30.0)+1.00*(declines/5.0)
    truth_probability=1.0/(1.0+__import__('math').exp(-truth_logit))
    risky=truth_probability>=0.58
    if random.random()<.015: risky=not risky
    return {"event_id":f"eval-{i}","amount_cents":amount,"currency":"USD","merchant_category":random.choice([5411,5812,5732,5999]),"country":country,"card_present":present,"hour_utc":hour,"velocity_5m":v5,"velocity_1h":v1,"prior_declines":declines,"label":int(risky)}

def main():
    OUT.parent.mkdir(parents=True,exist_ok=True)
    with OUT.open('w') as f:
        for i in range(2000): f.write(json.dumps(make(i),separators=(',',':'))+'\n')
    print(OUT)
if __name__=='__main__': main()

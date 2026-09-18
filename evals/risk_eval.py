import argparse, json, time
from pathlib import Path
import httpx

DATA=Path(__file__).parent/'data'/'risk_eval.jsonl'

def rule_baseline(e:dict)->int:
    return int(e['amount_cents']>400000 or e['velocity_5m']>=9 or e['prior_declines']>=4)

def metrics(y, p):
    tp=sum(a==1 and b==1 for a,b in zip(y,p)); fp=sum(a==0 and b==1 for a,b in zip(y,p)); fn=sum(a==1 and b==0 for a,b in zip(y,p)); tn=sum(a==0 and b==0 for a,b in zip(y,p))
    precision=tp/(tp+fp) if tp+fp else 0; recall=tp/(tp+fn) if tp+fn else 0; f1=2*precision*recall/(precision+recall) if precision+recall else 0; fpr=fp/(fp+tn) if fp+tn else 0
    return {'precision':precision,'recall':recall,'f1':f1,'false_positive_rate':fpr,'tp':tp,'fp':fp,'fn':fn,'tn':tn}

def main():
    p=argparse.ArgumentParser(); p.add_argument('--worker-url',default='http://localhost:8081'); a=p.parse_args()
    rows=[json.loads(x) for x in DATA.read_text().splitlines() if x.strip()]
    labels=[r.pop('label') for r in rows]
    baseline=[rule_baseline(r) for r in rows]
    preds=[]; lat=[]
    with httpx.Client(timeout=2.0) as c:
        for r in rows:
            t=time.perf_counter(); resp=c.post(a.worker_url+'/score',json=r); resp.raise_for_status(); lat.append((time.perf_counter()-t)*1000); preds.append(int(resp.json()['risky']))
    lat.sort(); pct=lambda p:lat[min(len(lat)-1,int(len(lat)*p))]
    print(json.dumps({'model':metrics(labels,preds),'baseline':metrics(labels,baseline),'http_inference_latency_ms':{'p50':pct(.5),'p95':pct(.95),'p99':pct(.99)}},indent=2))
if __name__=='__main__': main()
